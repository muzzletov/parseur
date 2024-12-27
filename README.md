# parseur

`parseur` is a simple HTML parser that allows for asynchronous hooks and preemptive cancelling of requests based on the evaluation of hooks.

## Features

- Asynchronous hooks
- Preemptive request cancellation
- Simple and intuitive API

## Installation

To install the `parseur` library, run:

```bash
go get github.com/muzzletov/parseur
```

## Usage

Here is a simple example of how to use `parseur`:

```go
package main

import (
	"github.com/muzzletov/parseur"
	"log"
)

func fetchOpenGraphTags() {
	client := parseur.NewClient()

	z := func(p *parseur.Parser) {
		exists := p.Query("head").First().Exists()

		if !exists { // this makes sure we get all the tags
			return
		}

		htmlTags := *p.Query("meta").Get()

		for _, u := range htmlTags {
			if token, ok := u.Attributes["property"]; ok && token == "og:video:tag" {
				p.InBound = func(i int) bool {
					return false
				}
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
```

## API at a Glance

### Parsing Functions

- `func NewEscapedParser(body *[]byte) *Parser`
- `func NewParser(body *[]byte, async bool, hook *func(p *Parser)) *Parser`
- `func (p *Parser) GetBody() []byte`
- `func (p *Parser) GetJoinedText(separator byte) string`
- `func (p *Parser) GetRoot() *Tag`
- `func (p *Parser) GetSize() int`
- `func (p *Parser) GetTagMap() map[string]struct{}`
- `func (p *Parser) GetTags(query string) *[]*Tag`
- `func (p *Parser) GetText() string`
- `func (p *Parser) Query(query string) *Query`

### Query Functions

- `func (q *Query) First() *QueryTag`
- `func (q *Query) Last() *QueryTag`
- `func (q *Query) Get() *[]*QueryTag`
- `func (q *Query) Query(query string) *Query`
- `func (qt *QueryTag) Query(query string) *Query`

### Web Client Functions

- `func NewClient() *WebClient`
- `func (c *WebClient) Fetch(url string) (*[]byte, error)`
- `func (c *WebClient) FetchParseAsync(request *Request) (p *Parser, err error)`
- `func (c *WebClient) FetchParseSync(request *Request) (p *Parser, err error)`
- `func (c *WebClient) FetchSync(request *Request) error`
- `func (c *WebClient) GetHttpClient() *http.Client`
- `func (c *WebClient) LoadCookies()`
- `func (c *WebClient) PersistCookies()`
- `func (c *WebClient) SetChunkSize(size int)`
- `func (c *WebClient) SetUserAgent(agent string)`

## Examples

For more examples, please refer to the `example` folder in the repository.
