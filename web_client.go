package parseur

import (
	"bufio"
	"io"
	"net/http"
)

type Request struct {
	RequestHeader  *http.Header
	ResponseHeader *http.Header
	Data           []byte
	Payload        *[]byte
	Url            *string
	Hook           *func(p *Parser)
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
		client:    &http.Client{Jar: jar},
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

func (c *WebClient) setup(u *string) (*http.Request, error) {
	req, err := http.NewRequest("GET", *u, nil)

	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)

	return req, nil
}

func (c *WebClient) Fetch(url string) (*[]byte, error) {
	req, err := c.setup(&url)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)

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

	request.Data, err = io.ReadAll(resp.Body)
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
	req, err := c.prepare(request)

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	request.Data, _ = io.ReadAll(resp.Body)
	request.ResponseHeader = &resp.Header

	parser := NewParser(&request.Data, false, nil)
	parser.Request = request
	return parser, nil
}

func (c *WebClient) GetHttpClient() *http.Client {
	return c.client
}

func (c *WebClient) prepare(request *Request) (*http.Request, error) {
	req, err := c.setup(request.Url)

	if err != nil {
		return nil, err
	}

	mergeHeaderFields(request.RequestHeader, &req.Header)

	return req, nil
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

	defer resp.Body.Close()

	buf := make([]byte, c.chunkSize)
	request.Data = make([]byte, 0)
	reader := bufio.NewReader(resp.Body)

	p = NewParser(&request.Data, true, request.Hook)
	p.Request = request

	var n = 0

	for !p.Done {
		n, err = reader.Read(buf)

		request.Data = append(request.Data, buf[:n]...)

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		select {
		case p.DataChan <- &request.Data:
		default:
		}
	}

	if !p.Done {
		*p.Complete = true
		p.DataChan <- &request.Data
		<-p.ParseComplete
	}

	return p, nil
}
