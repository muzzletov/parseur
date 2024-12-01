package parseur

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
)

type ExtJar struct {
	jar  *cookiejar.Jar
	cookies map[string][]*http.Cookie
}

func NewJar() *ExtJar {
	jar, err := cookiejar.New(nil)

	if err != nil {
		log.Fatal("Couldn't create a new cookie jar")
	}
	
	return &ExtJar{jar: jar, cookies: make(map[string][]*http.Cookie)}
}

func (j *ExtJar) Save(filename string) error {
	data, err := json.Marshal(j.cookies)

	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0600)
}

func (j *ExtJar) Load(filename string) error {
	data, err := os.ReadFile(filename)
	
	if err != nil {
		return err
	}
	
	var allCookies map[string][]*http.Cookie
	
	if err = json.Unmarshal(data, &allCookies); err != nil {
		return err
	}
	
	for urlString, cookies := range allCookies {
		u, err := url.Parse(urlString)
		if err != nil {
			return err
		}
		j.jar.SetCookies(u, cookies)
	}
	
	return nil
}


func (j *ExtJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.jar.Cookies(u)
}

func (j *ExtJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.String()] = cookies
	j.jar.SetCookies(u, cookies)
}

