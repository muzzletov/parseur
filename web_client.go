package parseur

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
)

type Request struct {
	RequestHeader  *http.Header
	ResponseHeader *http.Header
	Data           *[]byte
	Payload        *[]byte
	Url            *string
	Hook           *func(p *Parser)
	*context.CancelFunc
	Method string
}

type WebClient struct {
	chunkSize int
	client    *http.Client
	jar       *ExtJar
	userAgent string
}

func (c *WebClient) LoadCookies() {
	c.jar.Load("cookies.json")
}

func (c *WebClient) PersistCookies() {
	c.jar.Save("cookies.json")
}

func NewClient() *WebClient {
	jar := NewJar()
	return &WebClient{
		client: &http.Client{
			Jar: jar,
		},
		jar:       jar,
		chunkSize: 64000,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	}
}

func (c *WebClient) SetChunkSize(size int) {
	c.chunkSize = size
}

func (c *WebClient) SetUserAgent(agent string) {
	c.userAgent = agent
}

func (c *WebClient) setup(r *Request) (*http.Request, *context.CancelFunc, error) {
	if r.Url == nil {
		return nil, nil, errors.New("parseur: request has no URL")
	}

	var reader bytes.Reader
	var method = r.Method

	if method == "" {
		method = "GET"
	}

	if r.Payload != nil {
		reader = *bytes.NewReader(*r.Payload)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, method, *r.Url, &reader)

	if err != nil {
		cancel()
		return nil, nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)

	return req, &cancel, nil
}

func (c *WebClient) Fetch(url string) (*[]byte, error) {
	req, cancel, err := c.setup(&Request{Url: &url})

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		(*cancel)()
		return nil, err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	(*cancel)()
	return &data, err
}

func (c *WebClient) FetchSync(request *Request) error {
	req, err := c.prepare(request)

	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)

	request.Data = &data
	request.ResponseHeader = &resp.Header

	return err
}

func mergeHeaderFields(srcHeader *http.Header, dstHeader *http.Header) {
	if srcHeader == nil ||
		dstHeader == nil {
		return
	}

	for key, values := range *srcHeader {
		dstHeader.Del(key)

		for _, value := range values {
			dstHeader.Add(key, value)
		}
	}
}

func (c *WebClient) FetchParseSync(request *Request) (p *Parser, err error) {
	err = c.FetchSync(request)

	if request.Data == nil {
		return nil, err
	}

	parser := NewParser(request.Data, false, nil)
	parser.Request = request

	return parser, err
}

func (c *WebClient) GetHttpClient() *http.Client {
	return c.client
}

func (c *WebClient) prepare(request *Request) (*http.Request, error) {
	req, cancel, err := c.setup(request)

	if err != nil {
		return nil, err
	}

	mergeHeaderFields(request.RequestHeader, &req.Header)
	request.CancelFunc = cancel

	return req, nil
}

func (c *WebClient) FetchParseAsync(request *Request) (p *Parser, err error) {
	req, err := c.prepare(request)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		(*request.CancelFunc)()
		return nil, err
	}

	defer resp.Body.Close()

	request.ResponseHeader = &resp.Header

	buf := make([]byte, c.chunkSize)
	data := make([]byte, 0, 4*c.chunkSize)

	snapshot := func() *[]byte {
		view := data[:len(data):len(data)]
		return &view
	}

	p = NewParser(snapshot(), true, request.Hook)
	p.Request = request

	reader := bufio.NewReader(resp.Body)

	var readErr error

	for !p.isDone() {
		n, rerr := reader.Read(buf)

		if n > 0 {
			data = append(data, buf[:n]...)

			select {
			case p.DataChan <- snapshot():
			default:
			}
		}

		if rerr != nil {
			if rerr != io.EOF {
				readErr = rerr
			}

			break
		}
	}

	(*request.CancelFunc)()

	if p.isDone() {
		<-p.ParseComplete
	} else {
		p.setComplete()
		final := snapshot()

		for finished := false; !finished; {
			select {
			case p.DataChan <- final:
			case <-p.ParseComplete:
				finished = true
			}
		}
	}

	if readErr != nil {
		return nil, readErr
	}

	request.Data = snapshot()

	return p, nil
}
