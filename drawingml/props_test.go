package drawingml

import (
	"maps"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// changes are edits of Props a client may make, and what reading the
// element back gives.
var changes = map[string][]struct{ key, value, want string }{
	"run": {
		{"b", "1", "1"}, {"b", "", ""}, {"i", "0", "0"}, {"u", "dbl", "dbl"}, {"u", "bogus", ""},
		{"sz", "2400", "2400"}, {"sz", "99", ""}, {"baseline", "30000", "30000"},
		{"fill", `{"solid":{"scheme":"accent1","mods":[["lumMod",75000]]}}`, `{"solid":{"scheme":"accent1","mods":[["lumMod",75000]]}}`},
		{"fill", `{"none":true}`, `{"none":true}`},
		{"fill", `{"solid":{"rgb":"not a color"}}`, `{"solid":{"rgb":"000000"}}`},
		{"hl", `{"rgb":"FFFF00"}`, `{"rgb":"FFFF00"}`},
		{"font", "Carlito", "Carlito"}, {"font", "", ""}, {"ea", "+mn-ea", "+mn-ea"},
	},
	"para": {
		{"lvl", "2", "2"}, {"lvl", "9", ""}, {"algn", "ctr", "ctr"}, {"marL", "457200", "457200"},
		{"indent", "-228600", "-228600"}, {"lnSpc", "p90000", "p90000"}, {"spcBef", "t600", "t600"},
		{"spcAft", "x1", ""}, {"buClr", "tx", "tx"}, {"buClr", `{"scheme":"accent2"}`, `{"scheme":"accent2"}`},
		{"buSz", "p75000", "p75000"}, {"buFont", "Wingdings", "Wingdings"}, {"bu", "char:–", "char:–"},
		{"bu", "auto:romanUcPeriod:3", "auto:romanUcPeriod:3"}, {"bu", "auto:bogus:1", ""}, {"bu", "none", "none"},
		{"tabs", "l:914400 dec:1828800", "l:914400 dec:1828800"},
	},
	"body": {
		{"anchor", "ctr", "ctr"}, {"lIns", "0", "0"}, {"wrap", "none", "none"}, {"vert", "vert270", "vert270"},
		{"fit", "shape", "shape"}, {"fit", "norm", "norm"}, {"numCol", "2", "2"}, {"rot", "bad", ""},
	},
}

func TestPropsWriteWhatTheyRead(t *testing.T) {
	kinds := map[string]struct {
		read  func(*xmldom.Element) Props
		write func(e *xmldom.Element, old, props Props)
	}{
		"run":  {func(e *xmldom.Element) Props { return RunProps(e, nil) }, SetRunProps},
		"para": {func(e *xmldom.Element) Props { return readProps(e, paraProps, nil) }, SetParaProps},
		"body": {BodyProps, SetBodyProps},
	}
	elements := map[string]int{}
	presentationParts(t, func(file, name string, doc *xmldom.Document, _ []byte) {
		walk(doc.Root, func(e *xmldom.Element) {
			kind := map[string]string{"rPr": "run", "endParaRPr": "run", "defRPr": "run", "pPr": "para", "bodyPr": "body"}[e.Local]
			if kind == "" || e.Space != NS {
				return
			}
			elements[kind]++
			k := kinds[kind]
			old := k.read(e)
			delete(old, "link")
			before := string(e.Bytes())
			k.write(e, old, maps.Clone(old))
			if string(e.Bytes()) != before {
				t.Fatalf("%s %s: writing what was read changed %s into %s", file, name, before, e.Bytes())
			}
			if elements[kind]%97 != 0 {
				return
			}
			for _, c := range changes[kind] {
				props := maps.Clone(old)
				props[c.key] = c.value
				k.write(e, old, props)
				got := k.read(e)
				delete(got, "link")
				if got[c.key] != c.want {
					t.Fatalf("%s: %s set to %q reads %q in %s", before, c.key, c.value, got[c.key], e.Bytes())
				}
				for key := range props {
					dependent := c.key == "fit" && (key == "fontScale" || key == "lnSpcReduction")
					if key != c.key && !dependent && got[key] != old[key] {
						t.Fatalf("%s: setting %s changed %s to %q in %s", before, c.key, key, got[key], e.Bytes())
					}
				}
				old = got
			}
		})
	})
	t.Log(elements)
}
