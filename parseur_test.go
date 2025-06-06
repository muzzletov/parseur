package parseur

import (
	"log"
	"net/http"
	"net/url"
	"testing"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Test_ExtendedNestedQuery(t *testing.T) {
	payload := []byte(`<a class="rofl" id="a"><div></div><b><c><e><a><e></e><e class="lol">lol</e></a></e></c></b></a>`)
	p := NewParser(&payload, false, nil)
	tags := *p.Query("#a.rofl > b a > e.lol").Get()

	if len(tags) != 1 {
		panic("wrong length")
	}

	if tags[0].Name != "e" {
		panic("wrong tag")
	}

	if string(payload[tags[0].Body.Start:tags[0].Body.End]) != "lol" {
		panic("wrong innerhtml")
	}
}

func Test_QualifierSort(t *testing.T) {
	payload := []byte(``)
	p := NewParser(&payload, false, nil)
	qualifiers, _ := p.Query(".rofl#a > a").parseQualifiers(0)

	if len(*qualifiers) != 2 {
		panic("wrong qualifier length")
	}

	if (*qualifiers)[0] != "#a" {
		panic("wrong qualifier order")
	}

	if (*qualifiers)[1] != ".rofl" {
		panic("wrong qualifier order")
	}
}

func Test_ExtendedQuery(t *testing.T) {
	payload := []byte(`
		<a class="rofl" id="a">
			<div>
				<b></b>
			</div>Hi!
		</a>
		<div class="rofl" id="a">Hi!</div>
		How are you?
		<div class="lol">Bye.</div>
		<span id="a" class="rofl"></span>
	`)

	p := NewParser(&payload, false, nil)

	tags := p.Query("#a.rofl > b").Get()

	if tags != nil {
		panic("length doesnt match expected")
	}

	tags = p.Query("#a.rofl b").Get()

	if (*tags)[0].Name != "b" {
		panic("wrong tag")
	}

	tags = p.Query("#a.rofl div").Get()

	if (*tags)[0].Name != "div" {
		panic("wrong tag")
	}

	tags = p.Query("#a.rofl").Get()

	if (*tags)[0].Name != "a" {
		panic("wrong tag")
	}

	tags = p.Query("").Get()

	if tags != nil {
		panic("should return no result")
	}

	tags = p.Query("a").Get()

	if (*tags)[0].Name != "a" {
		panic("wrong tag")
	}

	tags = p.Query("div").Get()

	if len(*tags) != 3 {
		panic("tags has wrong length")
	}

	tags = p.Query("div.rofl").Get()

	if len(*tags) != 1 {
		panic("tags has wrong length")
	}
}

func Test_QueryTag(t *testing.T) {
	payload := []byte(`<div class="rofl" id="a">Hi!How are you?<div class="lol">Bye.</div><span id="a" class="rofl"></span></div>`)
	p := NewParser(&payload, false, nil)
	div := p.Query("#a").First()
	span := div.Query("span").First()

	if p.Query("#a > div").First().InnerText() != "Bye." {
		panic("wrong innertext")
	}

	if span.Name != "span" {
		panic("wrong element")
	}
}

func Test_QueryTagExtended(t *testing.T) {
	payload := []byte(`<div>a</div><div>b</div><div>c</div>`)
	p := NewParser(&payload, false, nil)
	divs := *p.Query("div").Get()

	if divs[0].InnerText() != "a" {
		panic("wrong element at offset")
	}

	if divs[1].InnerText() != "b" {
		panic("wrong element at offset")
	}

	if divs[2].InnerText() != "c" {
		panic("wrong element at offset")
	}

	payload = []byte(`<div><lol>a</lol></div><div><lol>b</lol></div><div><lol>c</lol></div>`)
	p = NewParser(&payload, false, nil)
	divs = *p.Query("div").Get()

	if divs[0].Query("lol").First().InnerText() != "a" {
		panic("wrong element at offset")
	}

	if divs[1].Query("lol").First().InnerText() != "b" {
		panic("wrong element at offset")
	}

	if divs[2].Query("lol").First().InnerText() != "c" {
		panic("wrong element at offset")
	}
}

func Test_IdQuery(t *testing.T) {
	payload := []byte(`<div class="rofl" id="a">Hi!</div>How are you?<div class="lol">Bye.</div><span id="a" class="rofl"></span>`)

	p := NewParser(&payload, false, nil)
	query := p.Query("#a")
	result := query.Get()

	if len(*result) != 1 || query.First().Name != "div" {
		log.Fatal("query result length incorrect")
	}
}

func Test_SubQueries(t *testing.T) {
	payload := []byte(`<div class="rofl" id="a"><yolo>Hi!</yolo></div>How are you?<div class="lol">Bye.</div><span id="a" class="rofl"></span>`)

	p := NewParser(&payload, false, nil)
	result := p.Query("#a").Query("yolo").Get()

	if len(*result) != 1 {
		log.Fatal("query result length incorrect")
	}

	if (*result)[0].Name != "yolo" {
		log.Fatal("query result tag incorrect")
	}
}

func Test_UnescapedTag(t *testing.T) {
	tl := []byte(`<a><p></a></p><br/>`)
	p := NewParser(&tl, false, nil)

	if p.Query("a").First().Children != nil {
		log.Fatal("element should not have successors")
	}

	tl = []byte(`<br<a>`)
	p = NewParser(&tl, false, nil)

	if p.Query("br").First().Exists() {
		log.Fatal("element should not exist")
	}
}

func Test_Invalid(t *testing.T) {
	t.Run("handle panic", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("Invalid query should panic")
			}
		}()

		tl := []byte(`<a><p></a></p><br/>`)
		p := NewParser(&tl, false, nil)
		p.Query("a + b").First()
	})
}

func Test_Query(t *testing.T) {
	payload := []byte(`<div class="rofl">Hi!</div>How are you?<div class="lol">Bye.</div><span class="rofl"></span>`)

	p := NewParser(&payload, false, nil)

	if p.GetRoot().Children[0] != p.Query("div").First().Tag ||
		p.GetRoot().Children[1] != p.Query("div").Last().Tag {
		log.Fatal("wrong elements returned")
	}

	if p.Query("span").First().Tag != p.Query("span").Last().Tag {
		log.Fatal("wrong elements returned")
	}

	if p.Query("body").First().Exists() {
		log.Fatal("wrong elements returned")
	}

	if p.Query(".rofl").Last().Tag != p.Query("span").Last().Tag {
		log.Fatal("wrong elements returned")
	}
}

func Test_Bounds(t *testing.T) {
	payload := "<div>fsdjkdksfdjskjkdfs</div>"
	body := []byte(payload)

	tag := NewParser(&body, false, nil).Query("div").First()

	if tag.OuterText() != payload[0:29] {
		log.Fatal("tag offset wrong")
	}

	payload = "<div></div>"
	body = []byte(payload)

	tag = NewParser(&body, false, nil).Query("div").First()

	if tag.OuterText() != payload[0:11] {
		log.Fatal("tag offset wrong")
	}

	payload = "<div />"
	body = []byte(payload)

	tag = NewParser(&body, false, nil).Query("div").First()

	if tag.OuterText() != "<div />" {
		log.Fatal("tag offset wrong")
	}
}

func Test_Body(t *testing.T) {
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

func Test_Classes(t *testing.T) {
	check := func(tags *[]*Tag, tag *Tag, ok bool) {
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

func Test_Value(t *testing.T) {
	html := []byte(``)
	c := NewParser(&html, false, nil)

	if c.value(0, 1) != "" {
		log.Fatal("out of bound request for substring should result in empty string being returned")
	}

	html = []byte(`invalid`)

	c = NewParser(&html, false, nil)

	if c.value(1, 0) != "" {
		log.Fatal("invalid range indices should result in empty string")
	}

	if c.value(-1, 0) != "" {
		log.Fatal("invalid range indices should result in empty string")
	}

	if c.value(0, -1) != "" {
		log.Fatal("invalid range indices should result in empty string")
	}
}

func Test_Extract(t *testing.T) {
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

	m := c.GetTagMap()
	l := []string{"fdjasjhfsadjh", "a", "z"}

	for _, i := range l {
		if _, ok := m[i]; !ok {
			log.Fatalf("tag %s not in set", i)
		}
	}

	if len(m) != 3 {
		log.Fatalf("wrong tag count")
	}

	html = []byte(`<a></a>`)
	c = NewParser(&html, false, nil)
	extract = c.GetJoinedText(' ')

	if extract != "" {
		log.Fatal("extracted text doesnt match")
	}
}

func Test_Attributes(t *testing.T) {
	l := []byte(`<bla><div attr="agfdgfdgfdgfd" z "yolo">lol</div></bla>`)
	c := NewParser(&l, false, nil)

	if c.Query("div").First().Attributes["attr"] != "agfdgfdgfdgfd" {
		log.Fatal("err")
	}

	if c.Query("div").First().Attributes["z"] != "z" {
		log.Fatal("err")
	}

	if c.Query("div").First().Attributes["yolo"] != "yolo" {
		log.Fatal("err")
	}
}

func Test_NewEscapedParser(t *testing.T) {
	l := []byte("<bla><div attr=\\\"agfdgfdgfdgfd\\\" z \\\"yolo\\\">lol</div></bla>")
	c := NewEscapedParser(&l)

	if c.Query("div").First().Attributes["attr"] != "agfdgfdgfdgfd" {
		log.Fatal("err")
	}

	if c.Query("div").First().Attributes["z"] != "z" {
		log.Fatal("err")
	}

	if c.Query("div").First().Attributes["yolo"] != "yolo" {
		log.Fatal("err")
	}
}

func Test_EscapedAttributes(t *testing.T) {
	attr := `{\"arr\":\"b\"}`
	data := []byte(`<div attr="` + attr + `"></div>`)
	c := NewParser(&data, false, nil)

	if c.Query("div").First().Attributes["attr"] != attr {
		log.Fatal("escaped attribute not parsed correctly")
	}
}

func Test_Wildcard(t *testing.T) {
	data := []byte(`<div attr="a"><li></li><a></a></div><p></p>`)
	c := NewParser(&data, false, nil)

	if len(*c.Query("*").Get()) != 4 {
		log.Fatal("wrong size for wildcard array")
	}
}

func Test_Attribute(t *testing.T) {
	data := []byte(`<div attr="a"></div>`)
	c := NewParser(&data, false, nil)

	if c.Query("div").First().Attributes["attr"] != `a` {
		log.Fatal("attribute not parsed correctly")
	}
}

func Test_CookieJar(t *testing.T) {
	cookieJar := NewJar()

	testURL, _ := url.Parse("https://example.com")
	cookies := []*http.Cookie{
		{Name: "test1", Value: "value1"},
		{Name: "test2", Value: "value2"},
	}

	cookieJar.SetCookies(testURL, cookies)

	saveFile := "test_cookies.json"
	err := cookieJar.Save(saveFile)
	if err != nil {
		t.Fatalf("Failed to save cookies: %v", err)
	}

	newCookieJar := NewJar()

	err = newCookieJar.Load(saveFile)
	if err != nil {
		t.Fatalf("Failed to load cookies: %v", err)
	}

	loadedCookies := newCookieJar.jar.Cookies(testURL)
	if len(loadedCookies) != len(cookies) {
		t.Fatalf("Expected %d cookies, got %d", len(cookies), len(loadedCookies))
	}

	for i, cookie := range cookies {
		if loadedCookies[i].Name != cookie.Name || loadedCookies[i].Value != cookie.Value {
			t.Errorf("Cookie mismatch: expected %s=%s, got %s=%s",
				cookie.Name, cookie.Value, loadedCookies[i].Name, loadedCookies[i].Value)
		}
	}
}

func Test_RequestHeaders(t *testing.T) {
	r, l := http.Header{}, http.Header{}

	r.Add("header", "b")
	r.Add("header", "c")

	mergeHeaderFields(&r, &l)

	if len(l["Header"]) != 2 ||
		l["Header"][0] != "b" ||
		l["Header"][1] != "c" {
		log.Fatal("merging headers did not work")
	}

}
