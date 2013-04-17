package main

import (
	"io"
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
	rv, err := parse(io.LimitReader(f, minRead), "x")
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}
	return rv
}

func TestCurrentValueGem(t *testing.T) {
	st := parseFile(t, "samples/normal.html")
	exp, err := bitcoin.AmountFromBitcoinsString("1.82")
	if err != nil {
		t.Fatalf("Error parsing expected amount:  %v", err)
	}
	if st.Value != exp {
		t.Errorf("Expected value %q, got %q", exp, st.Value)
	}
}

func TestCurrentValueBears(t *testing.T) {
	st := parseFile(t, "samples/bears.html")
	exp, err := bitcoin.AmountFromBitcoinsString("0.7379")
	if err != nil {
		t.Fatalf("Error parsing expected amount:  %v", err)
	}
	if st.Value != exp {
		t.Errorf("Expected value %q, got %q", exp, st.Value)
	}
}

func TestAddressParsing(t *testing.T) {
	a := `178sF7mXkMiGDKQWHjb1fVLEmzV4QSLrj`
	tests := []string{
		`{"address":"178sF7mXkMiGDKQWHjb1fVLEmzV4QSLrj"}`,
		`178sF7mXkMiGDKQWHjb1fVLEmzV4QSLrj`,
	}

	for _, test := range tests {
		got := parseAddress(test)
		if got != a {
			t.Errorf("Expected %q, got %q", a, got)
		}
	}
}
