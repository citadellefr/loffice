package loffice

import (
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/xlsx"
)

func TestWorkbooksAreCalculatedAndSaved(t *testing.T) {
	data, err := os.ReadFile("testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	store.data["book.xlsx"] = data
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "book.xlsx", Peer{ID: "1"})
	b, _, _ := join(t, h, "book.xlsx", Peer{ID: "2"})
	a.expect("join")

	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"cel","id":"S1","c":[[1,1,{"v":2}],[2,1,{"f":"A1*3"}],[3,1,{"f":"TEXT(A2,\"0,00\")"}]]}]}`)
	a.expect("ack")
	// the server follows with the values, for everyone
	for _, c := range []*client{a, b} {
		if c == b {
			c.expect("op")
		}
		f := c.expect("op")
		if f.SID != 0 || string(f.D) != `[{"o":"cel","id":"S1","c":[[2,1,{"v":6}],[3,1,{"v":"6,00"}]]}]` {
			t.Fatalf("follow-up: %+v %s", f, f.D)
		}
	}
	b.send(`{"t":"op","n":1,"v":2,"d":[{"o":"cel","id":"S1","c":[[1,1,{"v":5}]]}]}`)
	b.expect("ack")
	if f := b.expect("op"); string(f.D) != `[{"o":"cel","id":"S1","c":[[2,1,{"v":15}],[3,1,{"v":"15,00"}]]}]` {
		t.Fatalf("follow-up: %s", f.D)
	}
	// rows inserted: the server counts the moves the file saw
	a.expect("op")
	a.expect("op")
	a.send(`{"t":"op","n":2,"v":4,"d":[{"o":"ins","id":"S1","dim":"r","at":1,"n":1}]}`)
	a.expect("ack")
	if f := a.expect("op"); f.SID != 0 || !strings.Contains(string(f.D), `"moves":1`) {
		t.Fatalf("follow-up of rows inserted: %s", f.D)
	}
	<-store.saves
	_, tree, err := xlsx.Open([]byte(store.file("book.xlsx")))
	if err != nil {
		t.Fatal(err)
	}
	cells := map[[2]int]string{}
	for _, c := range tree.Node("S1").Grid.Cells() {
		cells[[2]int{c.Row, c.Col}] = string(c.Fields)
	}
	if cells[[2]int{3, 1}] != `{"f":"A2*3","v":15}` || cells[[2]int{4, 1}] != `{"f":"TEXT(A3,\"0,00\")","v":"15,00"}` {
		t.Fatalf("saved cells: %v", cells)
	}
	a.leave()
	b.leave()
}

func TestCSVFilesAreCalculatedAndSaved(t *testing.T) {
	store := newMemStore()
	store.data["ventes.csv"] = []byte("Article;Prix\r\nCafé;2,50\r\nThé;1,5\r\n")
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "ventes.csv", Peer{ID: "1"})
	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"cel","id":"S1","c":[[4,2,{"f":"SUM(B2:B3)"}]]}]}`)
	a.expect("ack")
	if f := a.expect("op"); string(f.D) != `[{"o":"cel","id":"S1","c":[[4,2,{"v":4}]]}]` {
		t.Fatalf("follow-up: %s", f.D)
	}
	a.send(`{"t":"op","n":2,"v":2,"d":[{"o":"new","id":"S2","t":"sheet","p":"book","k":"W","a":{"name":"Deux"},"c":[]}]}`)
	if f := a.expect("nack"); f.Error != xlsx.ErrCSVSheets.Error() {
		t.Fatalf("a second sheet: %+v", f)
	}
	<-store.saves
	if got := store.file("ventes.csv"); got != "Article;Prix\r\nCafé;2,50\r\nThé;1,5\r\n;4\r\n" {
		t.Fatalf("saved %q", got)
	}
	a.leave()
}
