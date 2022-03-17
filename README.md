- 👋 Hi, I’m @russian-boat-fuck-you
- 👀 I’m interested in fuck Putin's diplomacy
- 🌱 I’m currently learning yoha and dzen
- 💞️ I’m looking to collaborate on fuck russian boat to have a peace overall the world!
- 📫 How to reach me: I will found you when need

<!---
russian-boat-fuck-you/russian-boat-fuck-you is a ✨ special ✨ repository because its `README.md` (this file) appears on your GitHub profile.
You can click the Preview link to take a look at your changes.
--->

### (c) The initial idea is coming from https://github.com/grafov/hulk

#### Build an executable binary
    go build huilo.go

For raspberry PI 4 32-bit OS

    GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build  -trimpath -ldflags "-s -w -extldflags '-static'" -installsuffix cgo -tags netgo huilo.go
    
#### Usage
    ./huilo --help
    Usage of huilo:
        -t, --max-routines int          Maximum number of simultaneous connections (default 500)
        -p, --proxies-url proxies-url   URL to fetch proxy list from proxies-url (default "https://hutin-puy.nadom.app/proxy.json")
        -r, --refresh duration          Screen refresh interval in seconds (default 3s)
        -s, --site site-url             Sites list. Can be used multiple times. Have precedence over sites-url if set site-url
        -u, --sites-url sites-url       URL to fetch sites list from sites-url  (default "https://hutin-puy.nadom.app/sites.json")
              
    http_proxy=10.2.3.4:5678 https_proxy=10.2.3.5:5679 socks5_proxy=10.2.3.6:5680 ./huilo --max-routines 1000 --site http://178.248.237.238 --site http://185.170.2.62 -s https://185.170.2.62 -s http://195.208.109.88 --site https://195.208.109.88
