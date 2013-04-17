package main

import (
	"os"
	"testing"

	"github.com/dustin/go.bitcoin"
)

func parseFile(t *testing.T, fn string) State {
	f, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Error parsing sample: %v", err)
	}
	defer f.Close()
	rv, err := parse(f)
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}
	return rv
}

func TestCurrentValue(t *testing.T) {
	st := parseFile(t, "samples/normal.html")
	exp, err := bitcoin.AmountFromBitcoinsString("1.82")
	if err != nil {
		t.Fatalf("Error parsing expected amount:  %v", err)
	}
	if st.Value != exp {
		t.Errorf("Expected value %q, got %q", exp, st.Value)
	}
}
