package parseur

import (
	"log"
	"testing"
)

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
