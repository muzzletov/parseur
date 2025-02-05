package parseur

import (
	"bytes"
	"strings"
	"sync"
)

var scriptBytes = []byte{
	0x3c, 0x2f, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
}

const (
	FAILED  = -1
	PARSING = 0
)

var selfclosingTagsMap = map[string]struct{}{
	"meta":   {},
	"link":   {},
	"br":     {},
	"input":  {},
	"source": {},
	"hr":     {},
	"track":  {},
	"wbr":    {},
	"param":  {},
	"embed":  {},
	"col":    {},
	"base":   {},
	"area":   {},
}

type Offset struct {
	Start int
	End   int
}

type Parser struct {
	length        int
	lastIndex     int
	html          bool
	runAsync      bool
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
	GetOffsetList func() []*Tag
	Mu            sync.Mutex
	Request       *Request
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

func (p *Parser) value(start, end int) string {
	return string((*p.body)[start:end])
}

func MapFromTerms(text string) *map[string]struct{} {
	m := make(map[string]struct{})
	length := len(text)

	for i, k := 0, 0; i < length; i, k = i+1, i+1 {

		for ; i < length && text[i] != ' '; i++ {
		}

		if k != i {
			m[text[k:i]] = struct{}{}
		}
	}

	return &m
}

func (p *Parser) GetTagMap() map[string]struct{} {
	return *MapFromTerms(p.GetJoinedText(' '))
}

func (p *Parser) GetJoinedText(separator byte) string {
	builder := strings.Builder{}
	var reduce func(*Tag) = nil

	reduce = func(tag *Tag) {
		offset := tag.Body.Start

		for _, child := range tag.Children {
			length := child.Tag.Start - offset

			if length > 0 {
				builder.Write(p.GetBody()[offset : offset+length])
				builder.WriteByte(separator)
			}

			reduce(child)

			offset = child.Tag.End
		}

		if offset < tag.Body.End {
			builder.Write(p.GetBody()[offset:tag.Body.End])
			builder.WriteByte(separator)
		}
	}

	reduce(p.GetRoot())

	return builder.String()
}

func (p *Parser) sync(index int) bool {
	return p.length > index
}

func (p *Parser) async(index int) bool {
	if *p.Complete {
		p.InBound = p.sync

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
	parser.GetOffsetList = parser.computeOffsetList
	parser.current = &Tag{Children: make([]*Tag, 0), Name: "root"}
	parser.lastIndex = 0
	parser.root = parser.current
	parser.ffLiteral = parser.ffEscapedTagLiteral
	parser.length = len(*body)
	parser.InBound = parser.sync

	parser.parse()

	return parser
}

func NewParser(body *[]byte, async bool, hook *func(p *Parser)) *Parser {
	parser := createParser(body)
	parser.runAsync = async
	parser.hook = hook
	parser.GetOffsetList = parser.computeOffsetList
	parser.current = &Tag{Children: make([]*Tag, 0), Name: "root"}
	parser.lastIndex = 0
	parser.root = parser.current

	parser.ffLiteral = parser.ffTagLiteral

	if parser.runAsync {
		parser.DataChan = make(chan *[]byte)
		parser.ParseComplete = make(chan struct{})
		parser.InBound = parser.async

		go parser.parse()
	} else {
		parser.length = len(*body)
		parser.InBound = parser.sync

		parser.parse()
	}

	return parser
}

func (p *Parser) parse() {
	index := p.consumeNamespaceTag(p.skipWhitespace(0))

	if index == -1 {
		index = 0
	}

	_ = p.parseBody(index)

	if p.runAsync {
		p.Done = true
		select {
		case <-p.DataChan:
		default:
		}
		p.ParseComplete <- struct{}{}
	}
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
		notInBoundOrWrongTag := !p.InBound(z) || (*p.body)[z] != name[i]
		if notInBoundOrWrongTag {
			return -1
		}
	}

	isTagEnd := p.InBound(length) && (*p.body)[length] == '>'

	if isTagEnd {
		return length + 1
	}

	return -1
}

func (p *Parser) retrieveFromCache(index int) (int, bool) {
	tag, ok := p.offsetMap[index]

	if !ok {
		return -1, ok
	}

	currentIndex := index

	if tag.Tag.End == -1 {
		return -1, ok
	}

	if _, ok := selfclosingTagsMap[tag.Name]; ok {
		index = tag.Tag.End
	} else {
		index = p.parseTagEnd(tag.Tag.End, tag.Name)
	}

	currentIndex = p.skipWhitespace(index)

	if currentIndex != -1 {
		index = currentIndex
	}

	return index, ok
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
	index = currentIndex

	if _, ok := selfclosingTagsMap[self.Name]; ok {
		currentIndex = p.handleSelfclosing(currentIndex)

		if currentIndex == -1 {
			p.current = parent
			return index
		}
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

	parent.Children = append(parent.Children, self)

	if currentIndex == -1 {
		self.Body.End = -1
		currentIndex = index + 1

		if len(self.Children) > 0 {
			parent.Children = append(parent.Children, self.Children...)
		}

		self.Children = nil
	}

	self.Tag = Offset{Start: offset, End: currentIndex}

	p.offsetMap[offset] = self
	p.addTag(self.Name, self)
	p.addTag("*", self)
	p.current = parent

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

		isScriptEnd := bytes.Equal((*p.body)[index:index+8], scriptBytes)

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

func (p *Parser) addClasses(classes string) {
	length := len(classes)

	for i, k := 0, 0; i < length; i++ {
		for k = i; i < length && classes[i] == ' '; {
			i++
			k++
		}
		for i < length && classes[i] != ' ' {
			i++
		}

		id := "." + classes[k:i]
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
	isNotCorrectLiteral := literal != '"' && literal != '\''

	if isNotCorrectLiteral {
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
		p.GetOffsetList = func() []*Tag {
			return t
		}
	}

	return t
}

func (p *Parser) addId(value string, current *Tag) {
	queryHandle := "#" + value

	if _, ok := p.tagMap[queryHandle]; !ok {
		p.addTag(queryHandle, current)
	}
}
