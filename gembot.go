package main

import (
	"io"
	"log"
	"strings"

	"github.com/dustin/goquery"
)

const worthPrefix = "It is worth "

type State struct {
	Value string
}

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
	log.Printf("Got %q", txt)

	if txt[:len(worthPrefix)] == worthPrefix {
		parts := strings.SplitN(txt[len(worthPrefix):], " ", 2)
		rv.Value = parts[0]
	}

	return rv, nil
}

func main() {
}
