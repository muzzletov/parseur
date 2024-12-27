# parseur
a simple html parser that allows for async hooks
and preemptive cancelling of requests based
on what the hook's evaluates to.

see example folder for an example.

**API at a glance**:

func NewEscapedParser(body *[]byte) *Parser

func NewParser(body *[]byte, async bool, hook *func(p *Parser)) *Parser

func (p *Parser) GetBody() []byte

func (p *Parser) GetJoinedText(seperator byte) string

func (p *Parser) GetRoot() *Tag

func (p *Parser) GetSize() int

func (p *Parser) GetTagMap() map[string]struct{}

func (p *Parser) GetTags(query string) *[]*Tag

func (p *Parser) GetText() string

func (p *Parser) Query(query string) *Query

func (q *Query) First() *QueryTag

func (q *Query) Last() *QueryTag

func (q *Query) Get() *[]*QueryTag

func (q *Query) Query(query string) *Query

func (qt *QueryTag) Query(query string) *Query

func NewClient() *WebClient

func (c *WebClient) Fetch(url string) (*[]byte, error)

func (c *WebClient) FetchParseAsync(request *Request) (p *Parser, err error)

func (c *WebClient) FetchParseSync(request *Request) (p *Parser, err error)

func (c *WebClient) FetchSync(request *Request) error

func (c *WebClient) GetHttpClient() *http.Client

func (c *WebClient) LoadCookies()

func (c *WebClient) PersistCookies()

func (c *WebClient) SetChunkSize(size int)

func (c *WebClient) SetUserAgent(agent string)
