package main

import (
	"github.com/muzzletov/parseur"
	"log"
)

func fetchOpenGraphTags() {
	client := parseur.NewClient()

	z := func(p *parseur.Parser) {
		exists := p.Query("head").First().Exists()

		if !exists { // this makes sure we get all the relevant tags
			return
		}

		htmlTags := *p.Query("meta").Get()

		p.InBound = func(i int) bool {
			return false
		}

		for _, u := range htmlTags {
			if token, ok := u.Attributes["property"]; ok && token == "og:video:tag" {
				println(u.Attributes["content"])
			}
		}
	}

	u := "https://www.youtube.com/watch?v=pQO1t2Y627Y"
	_, err := client.FetchParseAsync(&parseur.Request{
		Url:  &u,
		Hook: &z,
	})

	if err != nil {
		log.Fatal(err.Error())
		return
	}
}

func main() {
	fetchOpenGraphTags()
}
