package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dustin/go.bitcoin"
	"github.com/dustin/goquery"
)

const minRead = 16384

var checkInterval = flag.Duration("interval", time.Minute, "How frequently to check")
var postBuyInterval = flag.Duration("postBuyInterval", time.Minute*5,
	"How long to wait after purchasing before checking again")

type State struct {
	IsMine bool
	Value  bitcoin.Amount
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
	BuyDisabled bool           `json:"disabled"`

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

func parse(r io.Reader, raddr string) (State, error) {
	rv := State{}

	g, err := goquery.Parse(r)
	if err != nil {
		return rv, err
	}
	txt := g.Find("h2").Text()

	worth := ""
	parts := strings.Split(txt, " ")
	for i, w := range parts {
		if w == "worth" && len(parts) > i {
			worth = parts[i+1]
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

	if s.BuyDisabled {
		log.Printf("Buy is disabled -- mocking it")
		s.markPurchased("MOCK")
		return true, nil
	}

	var txn string
	if s.FromAcct == "" {
		txn, err = bc.SendToAddress(x.Address, amt, s.Comment, "")
	} else {
		txn, err = bc.SendFrom(s.FromAcct, x.Address, amt, -1, s.Comment, "")
	}

	if err == nil {
		bought = true
		s.markPurchased(txn)
	}

	return
}

func (s *site) markPurchased(txn string) {
	log.Printf("Sent txn %v", txn)
	s.latestTx = txn

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

	// Don't bite off more than we can chew.
	threshold := s.Threshold
	balance, err := bc.GetBalance()
	if err != nil {
		return false, fmt.Errorf("Unable to get account balance: %v", err)
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

	if st.IsMine {
		log.Printf("I already seem to own it")
		return false, nil
	}

	if st.Value <= threshold {
		log.Printf("Hey, we'll give that a bid!")
		bought, err = s.buy(st.Value)
	}
	return
}

func (s site) monitor() {
	ticker := time.NewTicker(*checkInterval)
	var delay <-chan time.Time

	// not too happy with the copy and pasting here, but I want it
	// to run once, but still set these variables.
	bought, err := s.checkSite()
	if err != nil {
		log.Printf("Error checking %v: %v", s.ReadURL, err)
	}
	if bought {
		delay = time.After(*postBuyInterval)
	}

	for {
		t := ticker.C
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
				delay = time.After(*postBuyInterval)
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

	bc = bitcoin.NewBitcoindClient(conf.Bitcoin,
		conf.BitcoinUser, conf.BitcoinPass)

	go notify(conf.Notifications)

	for _, s := range conf.Sites {
		log.Printf("Doing %v", s.ReadURL)
		go s.monitor()
	}

	select {}
}
