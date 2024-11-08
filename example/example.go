package main

import (
	"github.com/muzzletov/parseur"
	"log"
)

func fetchOpenGraphTags() {
	client := parseur.NewClient()

	z := func(p *parseur.Parser) {
		q := p.First("head")

		if q == nil { // this makes sure we get all the tags
			return
		}

		htmlTags := p.Filter("meta")

		for _, u := range htmlTags {
			if token, ok := u.Attributes["property"]; ok && token == "og:video:tag" {
				p.InBound = func(i int) bool {
					return false
				}
				println(u.Attributes["content"])
			}

		}

	}

	_, err := client.FetchParseAsync("https://www.youtube.com/watch?v=pQO1t2Y627Y", &z)

	if err != nil {
		log.Fatal(err.Error())
		return
	}
}

func main() {
	fetchOpenGraphTags()
}
