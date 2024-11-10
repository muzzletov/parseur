package parseur

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

const (
	FAILED  = -1
	PARSING = 0
)

type ExtJar struct {
	jar  *cookiejar.Jar
	urls map[string]struct{}
}

func NewJar() *ExtJar {
	jar, _ := cookiejar.New(nil)
	return &ExtJar{jar: jar, urls: make(map[string]struct{})}
}

func (j *ExtJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.jar.Cookies(u)
}

func (j *ExtJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.urls[u.Path] = struct{}{}
	j.jar.SetCookies(u, cookies)
}

func NewClient() *WebClient {
	jar := NewJar()
	return &WebClient{
		client:    &http.Client{Jar: jar},
		jar:       jar,
		chunkSize: 64000,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	}
}

type WebClient struct {
	chunkSize int
	client    *http.Client
	jar       *ExtJar
	userAgent string
}

type Offset struct {
	Start int
	End   int
}

type Tag struct {
	Name       string
	Namespace  string
	Children   []*Tag
	Attributes map[string]string
	Body       Offset
}

type Parser struct {
	length        int
	lastIndex     int
	html          bool
	async         bool
	success       bool
	Done          bool
	root          *Tag
	current       *Tag
	namespaceTag  *Tag
	body          *[]byte
	Complete      *bool
	hook          *func(parser *Parser)
	DataChan      chan *[]byte
	ParseComplete chan struct{}
	offsetMap     map[int]*Tag
	namespaces    map[string]string
	tagMap        map[string][]*Tag
	InBound       func(int) bool
	OffsetList    func() []*Tag
	Mu            sync.Mutex
}

func (p *Parser) intersect(a *[]*Tag, b *[]*Tag) *[]*Tag {
	length := int(math.Min(float64(len(*a)), float64(len(*b))))
	tagMap := make(map[*Tag]struct{}, length)
	result := make([]*Tag, 0, length)

	for _, t := range *a {
		tagMap[t] = struct{}{}
	}

	for _, t := range *b {
		if _, ok := tagMap[t]; ok {
			result = append(result, t)
		}
	}

	return &result
}

func (c *WebClient) SetChunkSize(size int) {
	c.chunkSize = size
}

func (c *WebClient) SetUserAgent(agent string) {
	c.userAgent = agent
}

func (c *WebClient) FetchParseSync(url string) (p *Parser, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalln(err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		log.Fatalln(err)
	}

	defer resp.Body.Close()

	reader, _ := io.ReadAll(resp.Body)

	data := reader

	p = NewParser(&data, false, nil)

	return p, nil
}

func (c *WebClient) GetHttpClient() *http.Client {
	return c.client
}

func (c *WebClient) FetchParseAsync(url string, hook *func(p *Parser)) (p *Parser, err error) {

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalln(err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		log.Fatalln(err)
	}

	defer resp.Body.Close()

	buf := make([]byte, c.chunkSize)
	data := make([]byte, 0)

	reader := bufio.NewReader(resp.Body)

	p = NewParser(&data, true, hook)
	for !p.Done {
		r, err := reader.Read(buf)

		data = append(data, buf[:r]...)

		if err == io.EOF {
			break
		}

		if err != nil {
			fmt.Println(err)
			return nil, err
		}

		select {
		case p.DataChan <- &data:
		default:
		}
	}

	if !p.Done {
		*p.Complete = true
		p.DataChan <- &data
		<-p.ParseComplete
	}

	return p, nil
}

func (p *Parser) First(name string) *Tag {
	for _, tag := range p.offsetMap {
		if tag.Name == name {
			return tag
		}
	}

	return nil
}

func (p *Parser) Filter(name string) []*Tag {
	tags := make([]*Tag, 0)

	for _, tag := range p.offsetMap {
		if tag.Name == name {
			tags = append(tags, tag)
		}
	}

	return tags
}

func (t *Tag) FindAll(name string) []*Tag {
	children := make([]*Tag, 0)

	for _, c := range t.Children {
		if c.Name == name {
			children = append(children, c)
		}

		children = append(children, c.FindAll(name)...)
	}

	return children
}

func (p *Parser) Sync(index int) bool {
	return p.length > index
}

func (p *Parser) Async(index int) bool {
	if *p.Complete {
		p.InBound = p.Sync
		p.length = len(*p.body)

		return p.InBound(index)
	} else if p.length > index {
		return true
	}

	p.body = <-p.DataChan
	p.length = len(*p.body)

	if p.hook != nil {
		(*p.hook)(p)
	}

	return p.InBound(index)
}

func NewParser(body *[]byte, async bool, hook *func(p *Parser)) *Parser {
	complete := false
	parser := &Parser{
		offsetMap:  make(map[int]*Tag),
		body:       body,
		Complete:   &complete,
		Mu:         sync.Mutex{},
		Done:       false,
		namespaces: make(map[string]string),
		length:     len(*body),
		async:      async,
		hook:       hook,
		tagMap:     make(map[string][]*Tag),
	}

	parser.OffsetList = parser.computeOffsetList
	parser.current = &Tag{Children: make([]*Tag, 0), Name: "root"}
	parser.lastIndex = 0
	parser.root = parser.current

	if parser.async {
		parser.DataChan = make(chan *[]byte)
		parser.ParseComplete = make(chan struct{})
		parser.InBound = parser.Async

		go parser.parse()
	} else {
		parser.length = len(*body)
		parser.InBound = parser.Sync

		parser.parse()
	}

	return parser
}

func (p *Parser) parse() {
	index := p.consumeNamespaceTag(p.skipWhitespace(0))

	if index == -1 {
		index = 0
	}

	currentIndex := p.parseBody(index)

	p.success = currentIndex != -1

	if p.async {
		p.Done = true
		select {
		case <-p.DataChan:
		default:
		}
		p.ParseComplete <- struct{}{}
	}
}

func (p *Parser) Success() bool {
	return p.success
}

func (p *Parser) GetBody() []byte {
	return *p.body
}

func (p *Parser) GetSize() int {
	return p.length
}

func (p *Parser) GetRoot() *Tag {
	return p.root
}

func (p *Parser) parseDoctype(index int) int {

	if (*p.body)[index] != '!' {
		return -1
	}

	index = p.parseTagName(index + 1)

	if index == -1 {
		return -1
	}

	isNamespaceTag := (*p.body)[index] == '>'

	if isNamespaceTag {
		p.namespaceTag = p.current
	}

	p.html = strings.ToLower(p.current.Name) == "doctype" && p.current.Attributes["html"] == "html"

	return p.updatePointer(index + 1)
}

type Buffer struct {
	buffer *bytes.Buffer
	Mu     sync.Mutex
}

func (b *Buffer) Write(bytes []byte) {
	b.Mu.Lock()
	b.buffer.Write(bytes)
	b.Mu.Unlock()
}

func (b *Buffer) Read() []byte {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	return b.buffer.Bytes()
}

func (p *Parser) consumeNamespaceTag(index int) int {
	currentIndex := p.skipWhitespace(index)

	if !p.InBound(currentIndex) || currentIndex == -1 {
		return -1
	}
	if (*p.body)[currentIndex] != '<' {
		return -1
	}

	parent := p.current
	currentIndex++

	if p.InBound(currentIndex) && (*p.body)[currentIndex] != '?' {
		index = p.parseDoctype(currentIndex)
		p.current = parent
		return index
	}

	currentIndex++

	currentIndex = p.parseTagName(currentIndex)
	p.current = parent

	if currentIndex == -1 {
		return -1
	}

	isNamespaceTag := (*p.body)[currentIndex] == '?' && (*p.body)[currentIndex+1] == '>'

	if isNamespaceTag {
		p.namespaceTag = p.current
	}

	return p.updatePointer(currentIndex + 2)
}

func (p *Parser) updatePointer(currentIndex int) int {
	if p.lastIndex < currentIndex {
		p.lastIndex = currentIndex
	}

	return currentIndex
}

func (p *Parser) isWhitespace(index int) bool {
	r := (*p.body)[index]
	return r == ' ' || r == '\t' || r == '\n'
}

func (p *Parser) skipWhitespace(index int) int {
	if index == -1 || !p.InBound(index) {
		return -1
	}

	for p.InBound(index) && p.isWhitespace(index) {
		index++
	}

	return p.updatePointer(index)
}

func (p *Parser) parseTagEnd(index int, name string) int {

	if index == -1 || !p.InBound(index+1) {
		return -1
	}

	isNotEndTag := (*p.body)[index] != '<' || (*p.body)[index+1] != '/'

	if isNotEndTag {
		return -1
	}

	length := len(name) + index + 2

	for i, z := 0, index+2; z < length; i, z = i+1, z+1 {
		if !p.InBound(z) || (*p.body)[z] != name[i] {
			return -1
		}
	}

	if !p.InBound(length) || (*p.body)[length] != '>' {
		return -1
	}

	return p.updatePointer(length + 1)
}

func (p *Parser) isMETAorLINKtag(t *Tag) bool {
	return t.Name == "meta" ||
		t.Name == "link" ||
		t.Name == "img" ||
		t.Name == "input" ||
		t.Name == "source" ||
		t.Name == "br" ||
		t.Name == "hr"
}

func (p *Parser) consumeTag(index int) int {
	currentIndex := p.skipWhitespace(index)
	offset := currentIndex
	parent := p.current

	isOutOfBoundsOrNotStartOfTag := currentIndex == -1 ||
		!p.InBound(currentIndex) ||
		(*p.body)[currentIndex] != '<'

	if isOutOfBoundsOrNotStartOfTag {
		return -1
	}

	tag, ok := p.offsetMap[offset]

	if ok {
		if tag.Body.End == -1 {
			return -1
		}

		if p.isMETAorLINKtag(tag) {
			index = tag.Body.End
		} else {
			index = p.parseTagEnd(tag.Body.End, tag.Name)
		}

		currentIndex = p.skipWhitespace(index)

		if currentIndex != -1 {
			index = currentIndex
		}

		parent.Children = append(parent.Children, tag)
		p.current = parent

		return index
	}

	currentIndex = p.parseTagName(currentIndex + 1)
	self := p.current

	if currentIndex == -1 {
		return -1
	}

	isEndOfTag := p.InBound(currentIndex+1) && (*p.body)[currentIndex] == '/' && (*p.body)[currentIndex+1] == '>'

	if p.isMETAorLINKtag(self) {
		currentIndex = p.handleSelfclosing(currentIndex)
		p.offsetMap[offset] = self
		p.current.Body = Offset{offset, currentIndex}
	} else if isEndOfTag {
		p.offsetMap[offset] = self
		currentIndex += 2
	} else if (*p.body)[currentIndex] == '>' {
		p.offsetMap[offset] = self
		p.addTag(self.Name, self)

		index = currentIndex

		if self.Name != "script" {
			currentIndex = p.parseRegularBody(currentIndex)
		} else {
			currentIndex = p.ffScriptBody(currentIndex)
		}

		if currentIndex == -1 {
			p.current.Body = Offset{offset, -1}

			return index + 1
		}
	} else {
		return -1
	}

	parent.Children = append(parent.Children, self)
	p.current = parent

	return p.updatePointer(currentIndex)
}

func (p *Parser) addTag(id string, item *Tag) {
	if _, ok := p.tagMap[id]; ok {
		p.tagMap[id] = append(p.tagMap[id], item)
	} else {
		p.tagMap[id] = make([]*Tag, 1)
		p.tagMap[id][0] = item
	}
}

func (p *Parser) handleSelfclosing(index int) int {
	currentIndex := index
	currentIndex = p.skipWhitespace(currentIndex)

	if (*p.body)[currentIndex] == '>' {
		return currentIndex + 1
	}

	closedWithSlash :=
		p.InBound(currentIndex+1) && (*p.body)[currentIndex] == '/' && (*p.body)[currentIndex+1] == '>'

	if closedWithSlash {
		return currentIndex + 2
	}

	return -1
}

func (p *Parser) ffScriptBody(index int) int {
	start := index

	for index > -1 && p.InBound(index) {
		for p.InBound(index) && (*p.body)[index] != '<' {
			index++
		}

		if !p.InBound(index + 9) {
			return -1
		}

		isScriptEnd := bytes.Equal((*p.body)[index:index+8], []byte{
			0x3c, 0x2f, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
		})

		if isScriptEnd {
			k := p.skipWhitespace(index + 8)

			if k == -1 || (*p.body)[k] != '>' {
				index += 1
				continue
			}

			p.current.Body = Offset{start + 1, index}

			return k + 1
		}

		index += 1
	}

	return -1
}

func (p *Parser) parseRegularBody(index int) int {
	currentIndex := p.parseBody(index + 1)

	if currentIndex == -1 {
		return -1
	}

	index = currentIndex
	currentIndex = p.skipWhitespace(currentIndex)

	if currentIndex == -1 {
		return index
	}

	return currentIndex
}

func (p *Parser) LastPointer() int {
	return p.lastIndex
}

func (p *Parser) parseTagName(index int) int {
	currentIndex := index

	if !p.ffLetter(index) {
		return -1
	}

	if (*p.body)[index] == '"' || (*p.body)[index] == '\'' {
		index = p.checkForLiteral(index, (*p.body)[index])
	} else {
		index = p.skipValidTag(index)
	}

	current := &Tag{}

	if (*p.body)[index] == ':' {
		current.Namespace = string((*p.body)[currentIndex:index])
		currentIndex = index + 1
		index = p.skipValidTag(index + 1)
	}

	current.Name = string((*p.body)[currentIndex:index])
	p.current = current
	current.Attributes = make(map[string]string)
	currentIndex = p.skipWhitespace(index)

	if currentIndex != index {
		currentIndex = p.parseAttributes(currentIndex)
	}

	return p.updatePointer(currentIndex)
}

func (p *Parser) addOffsets(start int, end int) {
	p.current.Body = Offset{start, end}
}

func (p *Parser) parseBody(index int) int {
	if !p.InBound(index) {
		p.addOffsets(index, -1)
		return -1
	}

	offset := index
	currentIndex := index
	name := p.current.Name

	for index != -1 && p.InBound(index) {

		for p.InBound(index) && (*p.body)[index] != '<' {
			index++
		}

		currentIndex = index

		index = p.consumeComment(index)

		if index != -1 {
			continue
		}

		index = p.parseTagEnd(currentIndex, name)

		if index != -1 {
			p.addOffsets(offset, index)
			return index
		}

		index = p.consumeTag(currentIndex)

		if index != -1 {
			continue
		}

		index = currentIndex + 1
	}

	p.addOffsets(offset, -1)

	return -1
}

func (p *Parser) consumeComment(index int) int {
	if !p.InBound(index + 4) {
		return -1
	}

	hasStart :=
		(*p.body)[index] == '<' &&
			(*p.body)[index+1] == '!' &&
			(*p.body)[index+2] == '-' &&
			(*p.body)[index+3] == '-'

	if !hasStart {
		return -1
	}

	terminated := false

	for ; p.InBound(index+1) && !terminated; index++ {
		terminated =
			(*p.body)[index] == '-' &&
				(*p.body)[index+1] == '-' &&
				(*p.body)[index+2] == '>'
	}

	return index

}

func (p *Parser) parseAttributes(index int) int {
	currentIndex := index

	for currentIndex != -1 {
		var namespace *string = nil

		if (*p.body)[currentIndex] == '"' || (*p.body)[currentIndex] == '\'' {
			currentIndex = p.checkForLiteral(currentIndex, (*p.body)[currentIndex])
		} else {
			currentIndex = p.skipValidTag(currentIndex)
		}

		if currentIndex == -1 || !p.InBound(currentIndex) {
			return -1
		}

		name := string((*p.body)[index:currentIndex])

		if (*p.body)[currentIndex] == '>' {
			p.current.Attributes[name] = string((*p.body)[index:currentIndex])
			return p.updatePointer(currentIndex)
		}

		if name == "xmlns" &&
			(*p.body)[currentIndex] == ':' {
			index = currentIndex + 1
			currentIndex = p.skipValidTag(currentIndex + 1)

			if !p.InBound(currentIndex) {
				return -1
			}

			temp := string((*p.body)[index:currentIndex])
			namespace = &temp
		}

		if p.isWhitespace(currentIndex) {
			p.current.Attributes[name] = string((*p.body)[index:currentIndex])
			currentIndex = p.skipWhitespace(currentIndex + 1)
			continue
		}

		if (*p.body)[currentIndex] != '=' || !p.InBound(currentIndex+1) {
			return -1
		}

		literal := (*p.body)[currentIndex+1]

		if literal != '"' && literal != '\'' {
			return -1
		}

		currentIndex = currentIndex + 2
		index = currentIndex

		for p.InBound(currentIndex) && (*p.body)[currentIndex] != literal {
			currentIndex++
		}

		if !p.InBound(currentIndex) {
			return -1
		}

		if namespace != nil {
			p.namespaces[*namespace] = string((*p.body)[index:currentIndex])
		} else {
			p.current.Attributes[name] = string((*p.body)[index:currentIndex])
		}

		currentIndex = p.skipWhitespace(currentIndex + 1)
		index = currentIndex

		if currentIndex == -1 {
			return -1
		}

		if (*p.body)[currentIndex] == '?' ||
			(*p.body)[currentIndex] == '>' ||
			(*p.body)[currentIndex] == '/' && (*p.body)[currentIndex+1] == '>' {

			if attr, ok := p.current.Attributes["class"]; ok {
				p.addClasses(attr)
			}
			return p.updatePointer(index)
		}
	}

	return p.updatePointer(currentIndex)
}

func (p *Parser) addClasses(attr string) {
	length := len(attr)
	for i, k := 0, 0; i < length; i++ {
		for k = i; i < length && attr[i] == ' '; i, k = i+1, k+1 {
		}
		for ; i < length && attr[i] != ' '; i++ {
		}

		id := "." + attr[k:i]
		p.addTag(id, p.current)
	}
}

func (p *Parser) ffLetter(index int) bool {
	return p.InBound(index) && p.isAlpha(index)
}

func (p *Parser) isAlpha(index int) bool {
	r := (*p.body)[index]
	return ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func (p *Parser) checkForLiteral(index int, literal uint8) int {
	r := (*p.body)[index]

	if r != literal {
		return -1
	}

	index += 1

	for p.InBound(index) && (*p.body)[index] != 34 {
		index++
	}

	index += 1

	return p.updatePointer(index)
}

func (p *Parser) skipValidTag(index int) int {
	if !p.InBound(index) || !p.isValidTagStart(index) {
		return -1
	}

	index += 1

	for p.InBound(index) && p.isValidTagChar(index) {
		index++
	}

	return p.updatePointer(index)
}

func (p *Parser) isValidTagStart(index int) bool {
	r := (*p.body)[index]

	return ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

func (p *Parser) isValidTagChar(index int) bool {
	r := (*p.body)[index]
	return ('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || (r == '-')
}

func (p *Parser) computeOffsetList() []*Tag {
	t := make([]*Tag, 0, len(p.offsetMap))

	for _, tag := range p.offsetMap {
		t = append(t, tag)
	}

	if p.Done {
		p.OffsetList = func() []*Tag {
			return t
		}
	}
	return t
}
