package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dustin/go.bitcoin"
	"github.com/dustin/goquery"
)

const worthPrefix = "It is worth "

var threshold = bitcoin.Amount(bitcoin.SatoshisPerBitcoin / 10)

var siteread = flag.String("siteread", "http://bitcoingem.com/", "The site to monitor")
var sitebuy = flag.String("sitebuy", "http://bitcoingem.com/calls/create_new_address3.php",
	"URL to do the purchasing")
var recvaddr = flag.String("recvaddr", "", "A receive address for payouts")
var fromacct = flag.String("fromacct", "", "Sending account name")
var checkInterval = flag.Duration("interval", time.Minute, "How frequently to check")
var comment = flag.String("comment", "", "comment on your transaction")

var bcServer = flag.String("bitcoin", "http://localhost:8333/", "Bitcoind")
var bcUser = flag.String("bcuser", "someuser", "Bitcoin user")
var bcPass = flag.String("bcpass", "somepass", "Bitcoin password")

func init() {
	flag.Var(&threshold, "maxbid", "Maximum bid we'll place")
}

type State struct {
	IsMine bool
	Value  bitcoin.Amount
}

var unknownData = errors.New("I don't recognize the data")

var bc *bitcoin.BitcoindClient

func must(err error) {
	if err != nil {
		log.Fatalf("Unexpected error:  %v", err)
	}
}

func parse(r io.Reader) (State, error) {
	rv := State{}

	g, err := goquery.Parse(r)
	if err != nil {
		return rv, err
	}
	txt := g.Find("h2").Text()

	if txt[:len(worthPrefix)] == worthPrefix {
		parts := strings.SplitN(txt[len(worthPrefix):], " ", 2)
		rv.Value, err = bitcoin.AmountFromBitcoinsString(parts[0])
	} else {
		err = unknownData
	}

	rv.IsMine = strings.Contains(txt, *recvaddr)

	return rv, err
}

func buy(amt bitcoin.Amount) error {
	res, err := http.PostForm(*sitebuy, url.Values{"address": {*recvaddr}})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Failed to get payout address: %v", res.Status)
	}
	resdata, err := ioutil.ReadAll(io.LimitReader(res.Body, 80))
	if err != nil {
		return err
	}
	ress := strings.TrimSpace(string(resdata))

	x, err := bc.ValidateAddress(ress)
	if err != nil {
		return err
	}
	if !x.Isvalid {
		return fmt.Errorf("Returned an invalid address:  %v", ress)
	}

	var txn string
	if *fromacct == "" {
		txn, err = bc.SendToAddress(x.Address, amt, *comment, "")
	} else {
		txn, err = bc.SendFrom(*fromacct, x.Address, amt, -1, *comment, "")
	}

	if err == nil {
		log.Printf("Sent txn %v", txn)
	}

	return err
}

func checkSite() {
	res, err := http.Get(*siteread)
	must(err)
	defer res.Body.Close()
	st, err := parse(io.LimitReader(res.Body, 4096))
	must(err)
	log.Printf("Current state:  %+v", st)

	if st.IsMine {
		log.Printf("I already seem to own it")
		return
	}

	if st.Value <= threshold {
		log.Printf("Hey, we'll give that a bid!")
		err := buy(st.Value)
		must(err)
	}
}

func main() {
	flag.Parse()

	if *recvaddr == "" {
		log.Fatalf("You need to supply a receive addr")
	}

	bc = bitcoin.NewBitcoindClient(*bcServer, *bcUser, *bcPass)

	checkSite()
}
