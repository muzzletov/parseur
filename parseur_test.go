package parseur

import (
	"log"
	"testing"
)

func TestClasses(t *testing.T) {
	current := Tag{Attributes: map[string]string{"class": "a rofl lol rofl"}}
	parser := Parser{length: 12, tagMap: map[string][]*Tag{}, current: &current}
	parser.addClasses(current.Attributes["class"])
	tags, ok := parser.tagMap[".a"]
	check(tags, &current, ok)

	tags, ok = parser.tagMap[".lol"]
	check(tags, &current, ok)

	if len(parser.tagMap) != 3 {
		log.Fatal("map does not have correct size of elements")
	}
}

func check(tags []*Tag, tag *Tag, ok bool) {
	if !ok {
		log.Fatal("element not part of map")
	}

	for _, t := range tags {
		if t == tag {
			return
		}
	}

	log.Fatal("element not part of map")
}
