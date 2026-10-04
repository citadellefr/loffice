package loffice

import (
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/xlsx"

	"github.com/citadellefr/trame/trametest"
)

func TestWorkbooksAreCalculatedAndSaved(t *testing.T) {
	data, err := os.ReadFile("testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	store := trametest.NewStore()
	store.Data["book.xlsx"] = data
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "book.xlsx", Peer{ID: "1"})
	b, _, _ := join(t, h, "book.xlsx", Peer{ID: "2"})
	a.Expect("join")

	a.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"cel","id":"S1","c":[[1,1,{"v":2}],[2,1,{"f":"A1*3"}],[3,1,{"f":"TEXT(A2,\"0,00\")"}]]}]}`)
	a.Expect("ack")
	// the server follows with the values, for everyone
	for _, c := range []*trametest.Client{a, b} {
		if c == b {
			c.Expect("op")
		}
		f := c.Expect("op")
		if f.SID != 0 || string(f.D) != `[{"o":"cel","id":"S1","c":[[2,1,{"v":6}],[3,1,{"v":"6,00"}]]}]` {
			t.Fatalf("follow-up: %+v %s", f, f.D)
		}
	}
	b.Send(`{"t":"op","n":1,"v":2,"d":[{"o":"cel","id":"S1","c":[[1,1,{"v":5}]]}]}`)
	b.Expect("ack")
	if f := b.Expect("op"); string(f.D) != `[{"o":"cel","id":"S1","c":[[2,1,{"v":15}],[3,1,{"v":"15,00"}]]}]` {
		t.Fatalf("follow-up: %s", f.D)
	}
	// rows inserted: the server counts the moves the file saw
	a.Expect("op")
	a.Expect("op")
	a.Send(`{"t":"op","n":2,"v":4,"d":[{"o":"ins","id":"S1","dim":"r","at":1,"n":1}]}`)
	a.Expect("ack")
	if f := a.Expect("op"); f.SID != 0 || !strings.Contains(string(f.D), `"moves":1`) {
		t.Fatalf("follow-up of rows inserted: %s", f.D)
	}
	<-store.Saves
	_, tree, err := xlsx.Open([]byte(store.File("book.xlsx")))
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
	a.Leave()
	b.Leave()
}

func TestCSVFilesAreCalculatedAndSaved(t *testing.T) {
	store := trametest.NewStore()
	store.Data["ventes.csv"] = []byte("Article;Prix\r\nCafé;2,50\r\nThé;1,5\r\n")
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "ventes.csv", Peer{ID: "1"})
	a.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"cel","id":"S1","c":[[4,2,{"f":"SUM(B2:B3)"}]]}]}`)
	a.Expect("ack")
	if f := a.Expect("op"); string(f.D) != `[{"o":"cel","id":"S1","c":[[4,2,{"v":4}]]}]` {
		t.Fatalf("follow-up: %s", f.D)
	}
	a.Send(`{"t":"op","n":2,"v":2,"d":[{"o":"new","id":"S2","t":"sheet","p":"book","k":"W","a":{"name":"Deux"},"c":[]}]}`)
	if f := a.Expect("nack"); f.Error != xlsx.ErrCSVSheets.Error() {
		t.Fatalf("a second sheet: %+v", f)
	}
	<-store.Saves
	if got := store.File("ventes.csv"); got != "Article;Prix\r\nCafé;2,50\r\nThé;1,5\r\n;4\r\n" {
		t.Fatalf("saved %q", got)
	}
	a.Leave()
}
