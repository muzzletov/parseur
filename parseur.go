package parseur

import (
	"bufio"
	"bytes"
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

type Query struct {
	query  string
	tags   *[]*Tag
	parser *Parser
}

func (p *Parser) Query(query string) *Query {
	q := Query{query: query, parser: p}
	return &q
}

func (q *Query) Last() *Tag {
	tags := q.GetTags()

	if tags == nil {
		return nil
	}

	length := len(*tags)

	if length == 0 {
		return nil
	}

	return (*tags)[length-1]
}

func (q *Query) First() *Tag {
	tags := q.GetTags()

	if tags == nil || len(*tags) == 0 {
		return nil
	}
	return (*tags)[0]
}

func (q *Query) Intersect(query *Query) *Query {
	queryIntersection := Query{
		parser: q.parser,
		query:  q.query + " + " + query.query,
		tags:   GetIntersection(q.GetTags(), query.GetTags()),
	}

	return &queryIntersection
}

func (q *Query) GetTags() *[]*Tag {
	if q.tags == nil {
		q.execute()
	}
	return q.tags
}

func (q *Query) execute() *Query {
	if q.tags != nil {
		return q
	}

	if q.parser == nil {
		log.Fatal("q.parser == nil")
		return q
	}

	q.tags = q.parser.GetTags(q.query)

	return q
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
	Tag        Offset
}

type Parser struct {
	length        int
	lastIndex     int
	html          bool
	async         bool
	success       bool
	Done          bool
	root          *Tag
	ffLiteral     func(int) (int, *string)
	current       *Tag
	namespaceTag  *Tag
	body          *[]byte
	Complete      *bool
	hook          *func(parser *Parser)
	DataChan      chan *[]byte
	ParseComplete chan struct{}
	offsetMap     map[int]*Tag
	namespaces    map[string]string
	tagMap        map[string]*[]*Tag
	InBound       func(int) bool
	OffsetList    func() []*Tag
	Mu            sync.Mutex
}

func GetIntersection(a *[]*Tag, b *[]*Tag) *[]*Tag {
	if a == nil || b == nil {
		return nil
	}

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

func (c *WebClient) FetchSync(url string) (data []byte, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *WebClient) FetchParseSync(url string) (p *Parser, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
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
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	buf := make([]byte, c.chunkSize)
	data := make([]byte, 0)
	reader := bufio.NewReader(resp.Body)

	p = NewParser(&data, true, hook)

	var n = 0

	for !p.Done {
		n, err = reader.Read(buf)

		data = append(data, buf[:n]...)

		if err == io.EOF {
			break
		}

		if err != nil {
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

func (t *Tag) FindAll(name string) *[]*Tag {
	children := make([]*Tag, 0)

	for _, c := range t.Children {
		if c.Name == name {
			children = append(children, c)
		}

		children = append(children, *c.FindAll(name)...)
	}

	return &children
}

func (p *Parser) Sync(index int) bool {
	return p.length > index
}
func (p *Parser) GetText() string {
	builder := strings.Builder{}
	var reduce func(*Tag) = nil

	reduce = func(tag *Tag) {
		offset := tag.Body.Start

		for _, child := range tag.Children {
			length := child.Tag.Start - offset

			if length > 0 {
				builder.Write(p.GetBody()[offset : offset+length])
			}

			reduce(child)

			offset = child.Tag.End
		}

		if offset < tag.Body.End {
			builder.Write(p.GetBody()[offset:tag.Body.End])
			return
		}
	}

	reduce(p.GetRoot())

	return builder.String()
}

func (p *Parser) GetTagMap() map[string]struct{} {
	m := make(map[string]struct{})
	text := p.GetJoinedText(' ')
	length := len(text)

	for i, k := 0, 0; i < length; i, k = i+1, i+1 {

		for ; i < length && text[i] != ' '; i++ {
		}

		if k == i {
			continue
		}

		m[text[k:i]] = struct{}{}
	}

	return m
}

func (p *Parser) GetJoinedText(seperator byte) string {
	builder := strings.Builder{}
	var reduce func(*Tag) = nil

	reduce = func(tag *Tag) {
		offset := tag.Body.Start

		for _, child := range tag.Children {
			length := child.Tag.Start - offset

			if length > 0 {
				builder.Write(p.GetBody()[offset : offset+length])
				builder.WriteByte(seperator)
			}

			reduce(child)

			offset = child.Tag.End
		}

		if offset < tag.Body.End {
			builder.Write(p.GetBody()[offset:tag.Body.End])
			builder.WriteByte(seperator)
		}
	}

	reduce(p.GetRoot())

	return builder.String()
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

func createParser(body *[]byte) *Parser {
	complete := false
	return &Parser{
		offsetMap:  make(map[int]*Tag),
		body:       body,
		Complete:   &complete,
		Mu:         sync.Mutex{},
		Done:       false,
		namespaces: make(map[string]string),
		length:     len(*body),
		tagMap:     make(map[string]*[]*Tag),
	}
}

func NewEscapedParser(body *[]byte) *Parser {
	parser := createParser(body)
	parser.OffsetList = parser.computeOffsetList
	parser.current = &Tag{Children: make([]*Tag, 0), Name: "root"}
	parser.lastIndex = 0
	parser.root = parser.current
	parser.ffLiteral = parser.ffEscapedTagLiteral
	parser.length = len(*body)
	parser.InBound = parser.Sync

	parser.parse()

	return parser
}

func NewParser(body *[]byte, async bool, hook *func(p *Parser)) *Parser {
	parser := createParser(body)
	parser.async = async
	parser.hook = hook
	parser.OffsetList = parser.computeOffsetList
	parser.current = &Tag{Children: make([]*Tag, 0), Name: "root"}
	parser.lastIndex = 0
	parser.root = parser.current

	parser.ffLiteral = parser.ffTagLiteral

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
	parent := p.current
	index = p.parseTagName(index + 1)

	if index == -1 {
		return -1
	}

	isNamespaceTag := (*p.body)[index] == '>'

	if isNamespaceTag {
		p.namespaceTag = p.current
	}

	p.html = strings.ToLower(p.current.Name) == "doctype" && p.current.Attributes["html"] == "html"
	p.current = parent
	return index + 1
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

	if currentIndex == -1 || !p.InBound(currentIndex) || (*p.body)[currentIndex] != '<' {
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

	return currentIndex + 2
}

func (p *Parser) isWhitespace(index int) bool {
	return (*p.body)[index] == ' ' ||
		(*p.body)[index] == '\t' ||
		(*p.body)[index] == '\n' ||
		(*p.body)[index] == '\r'
}

func (p *Parser) skipWhitespace(index int) int {
	if index == -1 || !p.InBound(index) {
		return -1
	}

	for p.InBound(index) && p.isWhitespace(index) {
		index++
	}

	return index
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

	isTagEnd := p.InBound(length) && (*p.body)[length] == '>'

	if isTagEnd {
		return length + 1
	}

	return -1
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

func (p *Parser) retrieveFromCache(index int) (int, bool) {
	tag, ok := p.offsetMap[index]
	currentIndex := index

	if ok {
		if tag.Body.End == -1 {
			return -1, ok
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

		return index, ok
	}

	return -1, ok
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

	currentIndex, ok := p.retrieveFromCache(offset)

	if currentIndex != -1 {
		return currentIndex
	} else if ok {
		return -1
	}

	currentIndex = p.parseTagName(index + 1)
	self := p.current

	if currentIndex == -1 {
		p.current = parent
		return -1
	}

	isEndOfTag := p.InBound(currentIndex+1) && (*p.body)[currentIndex] == '/' && (*p.body)[currentIndex+1] == '>'

	if p.isMETAorLINKtag(self) {
		currentIndex = p.handleSelfclosing(currentIndex)
	} else if isEndOfTag {
		currentIndex += 2
	} else if (*p.body)[currentIndex] == '>' {
		index = currentIndex
		if self.Name == "script" {
			currentIndex = p.ffScriptBody(currentIndex)
		} else {
			currentIndex = p.parseRegularBody(currentIndex)
		}

	} else {
		return -1
	}

	if currentIndex == -1 {
		self.Body.End = -1
	}

	self.Tag = Offset{Start: offset, End: currentIndex}

	p.offsetMap[offset] = self
	p.addTag(self.Name, self)
	p.addTag("*", self)
	p.current = parent

	parent.Children = append(parent.Children, self)

	return currentIndex
}

func (p *Parser) addTag(id string, item *Tag) {
	if _, ok := p.tagMap[id]; ok {
		*p.tagMap[id] = append(*p.tagMap[id], item)
	} else {
		list := []*Tag{item}
		p.tagMap[id] = &list
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
	var value *string

	index, value = p.ffLiteral(index)

	if index == -1 {
		index, value = p.skipValidTag(currentIndex)
	}

	current := &Tag{}

	if (*p.body)[index] == ':' {
		current.Namespace = *value
		currentIndex = index + 1
		index, value = p.skipValidTag(index + 1)
	}

	current.Name = *value
	p.current = current
	current.Attributes = make(map[string]string)
	currentIndex = p.skipWhitespace(index)

	if currentIndex != index {
		currentIndex = p.parseAttributes(currentIndex)
	}

	return currentIndex
}

func (p *Parser) parseBody(index int) int {
	if !p.InBound(index) {
		p.current.addOffsets(index, -1)
		return -1
	}

	offset := index
	currentIndex := index
	self := p.current

	for index != -1 && p.InBound(index) {

		for p.InBound(index) && (*p.body)[index] != '<' {
			index++
		}

		currentIndex = index
		index = p.consumeComment(index)

		if index != -1 {
			continue
		}

		index = p.parseTagEnd(currentIndex, self.Name)

		if index != -1 {
			self.addOffsets(offset, currentIndex)
			return index
		}

		index = p.consumeTag(currentIndex)

		if index != -1 {
			continue
		}

		index = currentIndex + 1
	}

	self.addOffsets(offset, -1)

	return -1
}

func (t *Tag) addOffsets(start int, end int) {
	t.Body = Offset{start, end}
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

	if p.InBound(currentIndex+1) &&
		(*p.body)[currentIndex] == '/' &&
		(*p.body)[currentIndex+1] == '>' {
		return currentIndex
	}

	for currentIndex != -1 {
		var namespace *string = nil
		var value *string = nil

		c, value := p.ffLiteral(currentIndex)

		if c == -1 {
			c, value = p.skipValidTag(currentIndex)
		}

		currentIndex = c

		if currentIndex == -1 || !p.InBound(currentIndex) {
			return -1
		}

		name := *value

		if (*p.body)[currentIndex] == '>' {
			p.current.Attributes[name] = *value
			return currentIndex
		}

		if name == "xmlns" &&
			(*p.body)[currentIndex] == ':' {
			index = currentIndex + 1
			currentIndex, value = p.skipValidTag(currentIndex + 1)

			if !p.InBound(currentIndex) {
				return -1
			}

			namespace = value
		}

		if p.isWhitespace(currentIndex) {
			p.current.Attributes[name] = *value
			currentIndex = p.skipWhitespace(currentIndex + 1)
			continue
		}

		if (*p.body)[currentIndex] != '=' || !p.InBound(currentIndex+1) {
			return -1
		}

		index = currentIndex + 2

		currentIndex, value = p.ffLiteral(currentIndex + 1)

		if currentIndex == -1 {
			break
		}

		if namespace != nil {
			p.namespaces[*namespace] = *value
		} else {
			p.current.Attributes[name] = *value
		}

		currentIndex = p.skipWhitespace(currentIndex)
		index = currentIndex

		if currentIndex == -1 {
			break
		}

		if (*p.body)[currentIndex] == '>' ||
			((*p.body)[currentIndex] == '?' && (*p.body)[currentIndex+1] == '>') ||
			((*p.body)[currentIndex] == '/' && (*p.body)[currentIndex+1] == '>') {

			if attr, ok := p.current.Attributes["class"]; ok {
				p.addClasses(attr)
			}

			if attr, ok := p.current.Attributes["id"]; ok {
				p.addId(attr, p.current)
			}

			break
		}
	}

	return currentIndex
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

func (p *Parser) GetTags(query string) *[]*Tag {
	return p.tagMap[query]
}

func (p *Parser) ffLetter(index int) bool {
	return p.InBound(index) && p.isAlpha(index)
}

func (p *Parser) isAlpha(index int) bool {
	return ('A' <= (*p.body)[index] && (*p.body)[index] <= 'Z') ||
		('a' <= (*p.body)[index] && (*p.body)[index] <= 'z')
}

func (p *Parser) ffEscapedTagLiteral(index int) (int, *string) {
	currentIndex := index + 1

	if !p.InBound(currentIndex) || (*p.body)[index] != '\\' {
		return -1, nil
	}

	literal := (*p.body)[currentIndex]

	if literal != '"' && literal != '\'' {
		return -1, nil
	}

	for p.InBound(currentIndex+1) &&
		((*p.body)[currentIndex] != '\\' || (*p.body)[currentIndex+1] != literal) {
		if (*p.body)[currentIndex] == '\\' {
			currentIndex += 2
		} else {
			currentIndex++
		}
	}

	currentIndex += 1

	if !p.InBound(currentIndex) {
		return -1, nil
	}

	attrValue := string(*p.body)[index+2 : currentIndex-1]

	return currentIndex + 1, &attrValue
}

func (p *Parser) ffTagLiteral(index int) (int, *string) {

	if !p.InBound(index) {
		return -1, nil
	}

	currentIndex := index + 1
	literal := (*p.body)[index]

	if literal != '"' && literal != '\'' {
		return -1, nil
	}

	for p.InBound(currentIndex) && (*p.body)[currentIndex] != literal {
		if (*p.body)[currentIndex] == '\\' {
			currentIndex += 2
		} else {
			currentIndex++
		}
	}

	currentIndex += 1

	if !p.InBound(currentIndex) {
		return -1, nil
	}

	attrValue := string((*p.body)[index+1 : currentIndex-1])

	return currentIndex, &attrValue
}

func (p *Parser) skipValidTag(index int) (int, *string) {
	if !p.InBound(index) || !p.isValidTagStart(index) {
		return -1, nil
	}

	currentIndex := index + 1

	for p.InBound(currentIndex) && p.isValidTagChar(currentIndex) {
		currentIndex++
	}

	attrValue := string((*p.body)[index:currentIndex])

	return currentIndex, &attrValue
}

func (p *Parser) isValidTagStart(index int) bool {
	return ('A' <= (*p.body)[index] && (*p.body)[index] <= 'Z') ||
		('a' <= (*p.body)[index] && (*p.body)[index] <= 'z')
}

func (p *Parser) isValidTagChar(index int) bool {
	return ('0' <= (*p.body)[index] && (*p.body)[index] <= '9') ||
		('A' <= (*p.body)[index] && (*p.body)[index] <= 'Z') ||
		('a' <= (*p.body)[index] && (*p.body)[index] <= 'z') ||
		((*p.body)[index] == '-')
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

func (p *Parser) addId(attr string, current *Tag) {
	queryHandle := "#" + attr

	if _, ok := p.tagMap[queryHandle]; !ok {
		p.addTag(queryHandle, current)
	}
}
