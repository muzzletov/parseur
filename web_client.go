package parseur

import (
	"bufio"
	"bytes"
	"context"
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
	var reader bytes.Reader
	var method = r.Method

	if method == "" {
		method = "GET"
	}

	if r.Payload != nil {
		reader = *bytes.NewReader(*r.Payload)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, r.Method, *r.Url, &reader)

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

	for u, i := range *srcHeader {
		for _, z := range i {
			dstHeader.Add(u, z)
		}
	}
}

func (c *WebClient) FetchParseSync(request *Request) (p *Parser, err error) {
	err = c.FetchSync(request)

	if request.Data == nil {
		return nil, nil
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

func merge(old *[]byte, new *[]byte, length int, additionalLength int) *[]byte {
	if cap(*old) > length+additionalLength {
		*old = append(*old, (*new)[:additionalLength]...)

		return old
	}

	newLength := length + additionalLength
	var l = make([]byte, newLength, length+newLength)

	for i := 0; i < length; i++ {
		l[i] = (*old)[i]
	}

	for i := 0; i < additionalLength; i++ {
		l[i+length] = (*new)[i]
	}

	return &l
}

func (c *WebClient) FetchParseAsync(request *Request) (p *Parser, err error) {

	req, err := c.prepare(request)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}

	buf := make([]byte, c.chunkSize)
	data := make([]byte, 0, 4*c.chunkSize)
	dataPtr := &data
	reader := bufio.NewReader(resp.Body)
	length := 0

	p = NewParser(dataPtr, true, request.Hook)
	p.Request = request
	p.length = 0
	var n = 0

	for !p.Done {
		n, err = reader.Read(buf)

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		if n > 0 {
			dataPtr = merge(dataPtr, &buf, length, n)
		}

		length += n

		select {
		case p.DataChan <- dataPtr:
		default:
		}
	}

	err = resp.Body.Close()
	(*request.CancelFunc)()

	if !p.Done {
		*p.Complete = true
		p.DataChan <- dataPtr
		<-p.ParseComplete
	}

	request.Data = dataPtr

	return p, err
}
