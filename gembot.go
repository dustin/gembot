package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go.bitcoin"
	"github.com/dustin/goquery"
)

const minRead = 1024 * 16

const (
	normal = iota
	tooHigh
	owned
	aggressive
)

var durations = map[int]time.Duration{
	tooHigh:    time.Minute * 15,
	owned:      time.Minute * 5,
	normal:     time.Minute,
	aggressive: time.Second * 10,
}

type State struct {
	IsMine bool
	Value  bitcoin.Amount
}

var costFinders = []*regexp.Regexp{
	regexp.MustCompile(`It is worth ([\d.]+) bitcoins?`),
	regexp.MustCompile(`They are worth ([\d.]+) bitcoins?`),
	regexp.MustCompile(`re-homing fee is ([\d.]+) bitcoins?`),
}

var unknownData = errors.New("I don't recognize the data")

var bc *bitcoin.BitcoindClient

type site struct {
	Threshold   bitcoin.Amount `json:"threshold"`
	ReadURL     string         `json:"read"`
	BuyURL      string         `json:"buy"`
	RecvAddress string         `json:"recv"`
	FromAcct    string         `json:"fromacct"`
	Comment     string         `json:"comment"`
	Disabled    bool           `json:"disabled"`

	state       int
	latestTx    string
	previousAmt bitcoin.Amount
}

var conf = struct {
	Bitcoin     string `json:"bitcoin"`
	BitcoinUser string `json:"bcuser"`
	BitcoinPass string `json:"bcpass"`

	Sites         []site
	Notifications []notifier
}{}

const buyStateFile = ",buystate.json"

type buyIntent struct {
	site string
	amt  bitcoin.Amount
	res  chan bool
}

var buyReq = make(chan buyIntent)
var buyComplete = make(chan buyIntent)

func initAmounts() map[string]bitcoin.Amount {
	f, err := os.Open(buyStateFile)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Error opening buy state: %v", err)
	}
	defer f.Close()

	rv := map[string]bitcoin.Amount{}
	d := json.NewDecoder(f)
	err = d.Decode(&rv)
	if err != nil {
		log.Fatalf("Error decoding state: %v", err)
	}
	return rv
}

func persistState(st interface{}) {
	tmpfile := buyStateFile + ".tmp"
	f, err := os.Create(tmpfile)
	if err != nil {
		log.Printf("Error creating tmp file: %v", err)
		return
	}
	defer f.Close()

	e := json.NewEncoder(f)
	err = e.Encode(st)
	if err != nil {
		log.Printf("Error encoding state: %v", err)
	}

	os.Rename(tmpfile, buyStateFile)
}

func buyMonitor() {
	lastBuy := initAmounts()

	for {
		select {
		case req := <-buyReq:
			req.res <- lastBuy[req.site] != req.amt
		case req := <-buyComplete:
			lastBuy[req.site] = req.amt
			persistState(lastBuy)
			close(req.res)
		}
	}
}

func parse(r io.Reader, raddr string) (State, error) {
	rv := State{}

	g, err := goquery.Parse(r)
	if err != nil {
		return rv, err
	}

	locs := []string{"h2", "h3"}

	worth := ""
	txt := ""
	for _, loc := range locs {
		txt = g.Find(loc).Text()
		for _, r := range costFinders {
			m := r.FindAllStringSubmatch(txt, 2)
			if len(m) > 0 && len(m[0]) > 0 {
				worth = m[0][1]
				break
			}
		}
		if worth != "" {
			break
		}
	}
	if worth == "" {
		return rv, unknownData
	}
	rv.Value, err = bitcoin.AmountFromBitcoinsString(worth)

	rv.IsMine = strings.Contains(txt, raddr)

	return rv, err
}

func parseAddress(s string) string {
	if s[0] == '{' {
		ob := struct{ Address string }{}
		json.Unmarshal([]byte(s), &ob)
		return ob.Address
	}
	return s
}

func (s *site) buy(amt bitcoin.Amount) (bought bool, err error) {
	data := url.Values{"address": {s.RecvAddress}}
	req, err := http.NewRequest("POST", s.BuyURL, strings.NewReader(data.Encode()))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.ReadURL)
	req.Header.Set("Referer", s.ReadURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_8_3) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/27.0.1453.47 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false, fmt.Errorf("Failed to get payout address: %v", res.Status)
	}
	resdata, err := ioutil.ReadAll(io.LimitReader(res.Body, 80))
	if err != nil {
		return false, err
	}
	ress := parseAddress(strings.TrimSpace(string(resdata)))

	x, err := bc.ValidateAddress(ress)
	if err != nil {
		return false, err
	}
	if !x.Isvalid {
		return false, fmt.Errorf("Returned an invalid address:  %v", ress)
	}

	log.Printf("Sending %v to %v for %v", amt, x.Address, s.ReadURL)

	var txn string
	if s.FromAcct == "" {
		txn, err = bc.SendToAddress(x.Address, amt, s.Comment, "")
	} else {
		txn, err = bc.SendFrom(s.FromAcct, x.Address, amt, -1, s.Comment, "")
	}

	if err == nil {
		bought = true
		s.markPurchased(txn, amt)
	}

	return
}

func (s *site) markPurchased(txn string, amt bitcoin.Amount) {
	log.Printf("Sent txn %v", txn)
	s.latestTx = txn

	buyComplete <- buyIntent{s.ReadURL, amt, make(chan bool)}

	notifyCh <- notification{
		Event: "Purchased from " + s.ReadURL,
		Msg:   "Bought from " + s.ReadURL + " with " + txn,
	}
}

func (s *site) checkSite() (bought bool, err error) {
	defer func(start time.Time) {
		duration := time.Since(start)
		if duration > time.Second*5 {
			log.Printf("Took %v to check %v", duration, s.ReadURL)
		}
	}(time.Now())

	s.state = normal

	// Don't bite off more than we can chew.
	threshold := s.Threshold
	balance, err := bc.GetBalance()
	if err != nil {
		log.Printf("Unable to get account balance: %v", err)
		// return false, fmt.Errorf("Unable to get account balance: %v", err)
	}
	if threshold > balance {
		threshold = balance
	}

	req, err := http.NewRequest("GET", s.ReadURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%v", minRead))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	st, err := parse(io.LimitReader(res.Body, minRead), s.RecvAddress)
	if err != nil {
		return false, err
	}

	if st.Value != s.previousAmt {
		s.previousAmt = st.Value
		log.Printf("Value of %v is now:  %+v", s.ReadURL, st)
	}

	canch := make(chan bool)
	buyReq <- buyIntent{s.ReadURL, st.Value, canch}

	if !<-canch {
		log.Printf("Buy manager is blocking us from buying? (we own it?)")
		s.state = owned
		return false, nil
	}

	if st.IsMine {
		log.Printf("I already seem to own %v", s.ReadURL)
		s.state = owned
		return false, nil
	}

	if st.Value <= threshold {
		log.Printf("Hey, we'll give that a bid!")
		bought, err = s.buy(st.Value)
	}

	if bought || st.IsMine {
		s.state = owned
	} else if st.Value <= s.Threshold {
		s.state = aggressive
	} else {
		s.state = tooHigh
	}

	return
}

func (s site) randomDelay(n int) {
	d := time.Duration(rand.Intn(n)) * time.Second
	log.Printf("Waiting %v before starting timer of %v", d, s.ReadURL)
	time.Sleep(d)
}

func (s site) monitor() {
	tickers := map[int]*time.Ticker{
		tooHigh:    time.NewTicker(durations[tooHigh]),
		owned:      time.NewTicker(durations[owned]),
		normal:     time.NewTicker(durations[normal]),
		aggressive: time.NewTicker(durations[aggressive]),
	}
	var delay <-chan time.Time

	// not too happy with the copy and pasting here, but I want it
	// to run once, but still set these variables.
	bought, err := s.checkSite()
	if err != nil {
		log.Printf("Error checking %v: %v", s.ReadURL, err)
	}
	if bought {
		delay = time.After(durations[owned])
	}

	s.randomDelay(13)

	for {
		t := tickers[s.state].C
		if delay != nil {
			// If there's a delay, ignore our ticker
			t = nil
		}
		select {
		case <-delay:
			delay = nil
			log.Printf("Reenabling purchasing")
		case <-t:
			bought, err := s.checkSite()
			if err != nil {
				log.Printf("Error checking %v: %v", s.ReadURL, err)
			}
			if bought {
				delay = time.After(durations[owned])
			}
		}
	}
}

func readConf(fn string) {
	f, err := os.Open(fn)
	if err != nil {
		log.Fatalf("Error opening config: %v", err)
	}
	defer f.Close()

	d := json.NewDecoder(f)
	err = d.Decode(&conf)
	if err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	for _, v := range conf.Notifications {
		if _, ok := notifyFuns[v.Driver]; !ok {
			log.Fatalf("Unknown driver '%s' in '%s'", v.Driver, v.Name)
		}
	}
}

func main() {
	flag.Parse()

	readConf(flag.Arg(0))

	go buyMonitor()

	bc = bitcoin.NewBitcoindClient(conf.Bitcoin,
		conf.BitcoinUser, conf.BitcoinPass)

	go notify(conf.Notifications)

	for _, s := range conf.Sites {
		if s.Disabled {
			log.Printf("Ignoring %v since buy is disabled",
				s.ReadURL)
			continue
		}
		log.Printf("Doing %v", s.ReadURL)
		go s.monitor()
	}

	select {}
}
