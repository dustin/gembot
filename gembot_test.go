package main

import (
	"io"
	"os"
	"testing"

	"github.com/dustin/go.bitcoin"
)

const (
	myAddr = "n4pKTfuJLmbuK2PaymXLWGy3FEERTovmkK"
	myUrl  = "http://bleu.west.spy.net/~dustin/"
)

func parseFile(t *testing.T, fn string) (State, error) {
	f, err := os.Open(fn)
	if err != nil {
		return State{}, err
	}
	defer f.Close()
	return parse("http://whatever/", io.LimitReader(f, minRead),
		myAddr, myUrl)
}

func TestCurrentValue(t *testing.T) {
	tests := []struct {
		filename string
		exp      string
		isme     bool
		islocked bool
	}{
		{"normal.html", "1.82", false, false},
		{"locked.html", "1.82", false, true},
		{"mine.html", "1.82", true, false},
		{"mine2.html", "1.82", true, false},
		{"mine3.html", "1.82", true, false},
		{"bears.html", "0.7379", false, false},
		{"goldbar.html", "0.05", false, false},
		{"bitkitty.html", "0.2988", false, false},
		{"bitjade.html", "0.0625", false, false},
	}

	for _, test := range tests {
		st, err := parseFile(t, "samples/"+test.filename)
		if err != nil {
			t.Errorf("Error parsing sample from %v: %v", test.filename, err)
			continue
		}
		exp, err := bitcoin.AmountFromBitcoinsString(test.exp)
		if err != nil {
			t.Fatalf("Error parsing expected amount:  %v", err)
		}
		if st.Value != exp {
			t.Errorf("Expected value %q from %v, got %q",
				exp, test.filename, st.Value)
		}
		if st.IsMine != test.isme {
			t.Errorf("Expected isme=%v for %v",
				test.isme, test.filename)
		}
		if st.Locked != test.islocked {
			t.Errorf("Expected locked=%v for %v",
				test.islocked, test.filename)
		}
	}
}

func TestPending(t *testing.T) {
	exp := `5c25cc9586571c1327961b22203652cab50b24b82dc4e5091fb5b5821df03061`
	st, err := parseFile(t, "samples/pending.html")
	if err != nil {
		t.Fatalf("Error parsing pending sample")
	}
	if !st.Locked {
		t.Fatalf("Expected locked")
	}
	if st.Pending != exp {
		t.Fatalf("Expected tx=%q, got %q", exp, st.Pending)
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
