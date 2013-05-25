package main

import (
	"encoding/csv"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type doubleslice [][]string

func (p doubleslice) Len() int           { return len(p) }
func (p doubleslice) Less(i, j int) bool { return p[i][0] < p[j][0] }
func (p doubleslice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Sort is a convenience method.
func (p doubleslice) Sort() { sort.Sort(p) }

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

	var tlist doubleslice

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
			tlist = append(tlist, []string{
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

	tlist.Sort()

	e.WriteAll([][]string(tlist))

	e.Flush()
}

func startHTTPServer(addr string) {
	http.HandleFunc("/export.csv", exportTransactions)
	log.Fatal(http.ListenAndServe(addr, nil))
}
