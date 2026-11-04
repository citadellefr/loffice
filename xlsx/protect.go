package xlsx

import (
	"encoding/json"
	"slices"

	"github.com/citadellefr/loffice/internal/xmldom"
)

const protectKey = "protect"

// Protect is the protection of a sheet, its "protect": what the locked
// cells and the objects of a protected sheet cannot be given.
type Protect struct {
	// Pass is the hash of the password in hexadecimal as Excel 97 made
	// it, none when the sheet is open to all.
	Pass      string `json:"pass,omitempty"`
	Objects   bool   `json:"objects,omitempty"`
	Scenarios bool   `json:"scenarios,omitempty"`
	// Allow names what a protected sheet leaves to its users, among
	// protectAllowed.
	Allow []string `json:"allow,omitempty"`
}

// protectAllowed are what a protected sheet may allow, with what the
// attribute of the sheet says when it does not: they are locked, but for
// the selections, which are free.
var protectAllowed = []struct {
	name   string
	locked string
}{
	{"formatCells", "1"}, {"formatColumns", "1"}, {"formatRows", "1"}, {"insertColumns", "1"}, {"insertRows", "1"},
	{"insertHyperlinks", "1"}, {"deleteColumns", "1"}, {"deleteRows", "1"}, {"selectLockedCells", "0"}, {"sort", "1"},
	{"autoFilter", "1"}, {"pivotTables", "1"}, {"selectUnlockedCells", "0"},
}

func readProtect(root *xmldom.Element) *Protect {
	e := root.Child(mainNS, "sheetProtection")
	if e == nil || !truthy(e.Get("sheet")) {
		return nil
	}
	p := &Protect{Pass: e.Get("password"), Objects: truthy(e.Get("objects")), Scenarios: truthy(e.Get("scenarios"))}
	for _, a := range protectAllowed {
		v, set := e.Attr(a.name)
		if !set {
			v = a.locked
		}
		if !truthy(v) {
			p.Allow = append(p.Allow, a.name)
		}
	}
	return p
}

func writeProtect(root *xmldom.Element, raw json.RawMessage) {
	if old := root.Child(mainNS, "sheetProtection"); old != nil {
		root.Remove(old)
	}
	var p Protect
	if json.Unmarshal(raw, &p) != nil || len(raw) == 0 || string(raw) == "null" {
		return
	}
	e := child(root, "sheetProtection", worksheetOrder)
	setAttr(e, "password", p.Pass)
	e.Set("sheet", "1")
	setAttr(e, "objects", boolAttr(p.Objects))
	setAttr(e, "scenarios", boolAttr(p.Scenarios))
	for _, a := range protectAllowed {
		want := "1"
		if slices.Contains(p.Allow, a.name) {
			want = "0"
		}
		if want != a.locked {
			e.Set(a.name, want)
		}
	}
}
