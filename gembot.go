package main

import (
	"io"
	"log"
	"os"

	"github.com/opesun/goquery"
)

func must(err error) {
	if err != nil {
		log.Fatalf("Unexpected error:  %v", err)
	}
}

func parse(r io.Reader) error {
	g, err := goquery.Parse(r)
	if err != nil {
		return err
	}
	t := g.Find("h2").Text()
	log.Printf("Got %q", t)
	return nil
}

func main() {
	f, err := os.Open(os.Args[1])
	must(err)
	defer f.Close()
	must(parse(f))
}
