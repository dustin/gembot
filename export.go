package main

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
	"time"
)

func exportTransactions(w http.ResponseWriter, req *http.Request) {
	txns, err := bc.ListTransactions("", 1000, 0)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.WriteHeader(200)

	e := csv.NewWriter(w)

	e.Write([]string{"ts", "comment", "confirmations", "amount", "fee"})

	for _, t := range txns {
		if t.Amount > 0 {
			// Ignore funding, we only care about what we paid out.
			continue
		}
		e.Write([]string{
			t.TransactionTime().Format(time.RFC3339),
			t.Comment,
			strconv.Itoa(t.Confirmations),
			t.Amount.String(),
			t.Fee.String(),
		})
	}
	e.Flush()
}

func startHTTPServer(addr string) {
	http.HandleFunc("/export", exportTransactions)
	log.Fatal(http.ListenAndServe(addr, nil))
}