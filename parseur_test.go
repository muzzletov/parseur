package parseur

import (
	"log"
	"testing"
)

func TestQuery(t *testing.T) {
	payload := []byte(`<div class="rofl">Hi!</div>How are you?<div class="lol">Bye.</div><span class="rofl"></span>`)

	p := NewParser(&payload, false, nil)
	result := p.Query("div").Intersect(p.Query(".lol")).GetTags()

	if result == nil ||
		len(*result) != 1 {
		log.Fatal("query result length incorrect")
	}

	if p.GetRoot().Children[0] != p.Query("div").First() ||
		p.GetRoot().Children[1] != p.Query("div").Last() {
		log.Fatal("wrong elements returned")
	}

	if p.Query("span").First() != p.Query("span").Last() {
		log.Fatal("wrong elements returned")
	}

	if p.Query("body").First() != nil {
		log.Fatal("wrong elements returned")
	}

	if p.Query(".rofl").Last() != p.Query("span").Last() {
		log.Fatal("wrong elements returned")
	}
}

func TestIntersection(t *testing.T) {
	a := Tag{Name: "a"}
	b := Tag{Name: "b"}
	c := Tag{Name: "c"}
	firstList := []*Tag{&a}
	secondList := []*Tag{&b, &c}
	if len(*GetIntersection(&firstList, &secondList)) != 0 {
		log.Fatal("wrong array size")
	}

	secondList = append(secondList, &a)
	result := *GetIntersection(&firstList, &secondList)

	if len(result) != 1 {
		log.Fatal("wrong array size")
	}

	if result[0].Name != "a" {
		log.Fatal("wrong resulting element")
	}
}

func TestBounds(t *testing.T) {
	payload := "<div>fsdjkdksfdjskjkdfs</div>"
	body := []byte(payload)

	tag := NewParser(&body, false, nil).First("div")

	if payload[(*tag).Tag.Start:(*tag).Tag.End] != payload[0:29] {
		log.Fatal("tag offset wrong")
	}

	payload = "<div></div>"
	body = []byte(payload)

	tag = NewParser(&body, false, nil).First("div")

	if payload[(*tag).Tag.Start:(*tag).Tag.End] != payload[0:11] {
		log.Fatal("tag offset wrong")
	}

	payload = "<div />"
	body = []byte(payload)

	tag = NewParser(&body, false, nil).First("div")

	if payload[(*tag).Tag.Start:(*tag).Tag.End] != "<div />" {
		log.Fatal("tag offset wrong")
	}
}

func TestBody(t *testing.T) {
	payload := "<div>fsdjkdksfdjskjkdfs</div>"
	body := []byte(payload)

	tag := NewParser(&body, false, nil).First("div")

	if payload[(*tag).Body.Start:(*tag).Body.End] != payload[5:23] {
		log.Fatal("payload offset wrong")
	}

	payload = "<div></div>"
	body = []byte(payload)

	tag = NewParser(&body, false, nil).First("div")

	if payload[(*tag).Body.Start:(*tag).Body.End] != "" {
		log.Fatal("payload offset wrong")
	}

	payload = "<div />"
	body = []byte(payload)

	p := NewParser(&body, false, nil)
	tag = p.First("div")
	if payload[(*tag).Body.Start:(*tag).Body.End] != "" || (*tag).Body.End != 0 {
		log.Fatal("payload offset wrong")
	}
}

func TestClasses(t *testing.T) {
	current := Tag{Attributes: map[string]string{"class": "a rofl lol rofl"}}
	parser := Parser{length: 12, tagMap: map[string]*[]*Tag{}, current: &current}
	parser.addClasses(current.Attributes["class"])
	tags, ok := parser.tagMap[".a"]
	check(tags, &current, ok)

	tags, ok = parser.tagMap[".lol"]
	check(tags, &current, ok)

	if len(parser.tagMap) != 3 {
		log.Fatal("map does not have correct size of elements")
	}
}

func check(tags *[]*Tag, tag *Tag, ok bool) {
	if !ok {
		log.Fatal("element not part of map")
	}

	for _, t := range *tags {
		if t == tag {
			return
		}
	}

	log.Fatal("element not part of map")
}

func TestExtract(t *testing.T) {
	html := []byte(`<a>fdjasjhfsadjh<div>a<HAHAHA>z</HAHAHA></div><p></p></a>`)
	c := NewParser(&html, false, nil)
	extract := c.GetText()
	if extract != "fdjasjhfsadjhaz" {
		log.Fatal("extracted text doesnt match")
	}

	extract = c.GetJoinedText(' ')
	if extract != "fdjasjhfsadjh a z " {
		log.Fatal("extracted text doesnt match")
	}

	html = []byte(`<a></a>`)
	c = NewParser(&html, false, nil)
	extract = c.GetJoinedText(' ')
	if extract != "" {
		log.Fatal("extracted text doesnt match")
	}
}

func TestEscapedAttributes(t *testing.T) {
	attr := `{\"arr\":\"b\"}`
	data := []byte(`<div attr="` + attr + `"></div>`)
	c := NewParser(&data, false, nil)

	if (*c.GetTags("div"))[0].Attributes["attr"] != attr {
		log.Fatal("escaped attribute not parsed correctly")
	}
}

func TestWildcard(t *testing.T) {
	data := []byte(`<div attr="a"><li></li><a></a></div><p></p>`)
	c := NewParser(&data, false, nil)

	if len(*c.GetTags("*")) != 4 {
		log.Fatal("wrong size for wildcard array")
	}
}

func TestAttribute(t *testing.T) {
	data := []byte(`<div attr="a"></div>`)
	c := NewParser(&data, false, nil)

	if (*c.GetTags("div"))[0].Attributes["attr"] != `a` {
		log.Fatal("attribute not parsed correctly")
	}
}
