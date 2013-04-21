package main

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func exportTransactions(w http.ResponseWriter, req *http.Request) {
	q := req.FormValue("q")

	after, _ := parseTime(req.FormValue("after"))

	accts, err := bc.ListAccounts()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.WriteHeader(200)

	e := csv.NewWriter(w)

	e.Write([]string{"ts", "acct", "dir", "comment",
		"confirmations", "amount", "fee", "txn"})

	for acct := range accts {
		txns, err := bc.ListTransactions(acct, 1000, 0)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		for _, t := range txns {
			if !(strings.Contains(acct, q) || strings.Contains(t.Comment, q)) {
				continue
			}
			if !t.TransactionTime().After(after) {
				continue
			}
			dir := "out"
			if t.Amount > 0 {
				dir = "in"
			}
			e.Write([]string{
				t.TransactionTime().Format(time.RFC3339),
				acct,
				dir,
				t.Comment,
				strconv.Itoa(t.Confirmations),
				t.Amount.String(),
				t.Fee.String(),
				t.TXID,
			})
		}
	}
	e.Flush()
}

func startHTTPServer(addr string) {
	http.HandleFunc("/export.csv", exportTransactions)
	log.Fatal(http.ListenAndServe(addr, nil))
}
