package drawingml

import (
	"bytes"
	"encoding/json"
	"maps"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
)

var shapeChanges = []struct{ key, value string }{
	{"xfrm", `{"x":10,"y":20,"w":300,"h":400,"rot":5400000,"flipH":true}`},
	{"xfrm", `{"x":0,"y":0,"w":1,"h":1}`},
	{"geom", `{"prst":"roundRect","av":{"adj":"val 20000"}}`},
	{"geom", `{"cust":{"rect":["l","t","r","b"],"paths":[{"w":100,"h":100,"d":[["M","0","0"],["L","100","100"],["A","50","50","0","5400000"],["Z"]]}]}}`},
	{"fill", `{"solid":{"rgb":"FF0000"}}`},
	{"fill", `{"grad":{"stops":[[0,{"scheme":"accent1"}],[100000,{"scheme":"accent2","mods":[["alpha",50000]]}]],"lin":5400000,"scaled":true}}`},
	{"fill", `{"patt":{"prst":"dkDnDiag","fg":{"rgb":"000000"},"bg":{"rgb":"FFFFFF"}}}`},
	{"fill", `{"none":true}`},
	{"line", `{"w":12700,"fill":{"solid":{"scheme":"tx1"}},"dash":"sysDash","join":"round","tail":{"type":"triangle"}}`},
	{"line", `{"fill":{"none":true}}`},
}

func TestShapePropsWriteWhatTheyRead(t *testing.T) {
	n := 0
	presentationParts(t, func(file, name string, doc *xmldom.Document, _ []byte) {
		walk(doc.Root, func(e *xmldom.Element) {
			if e.Local != "spPr" {
				return
			}
			n++
			if n%13 != 0 {
				return
			}
			old := ShapeProps(e, nil)
			for _, c := range shapeChanges {
				values := maps.Clone(old)
				values[c.key] = json.RawMessage(c.value)
				SetShapeProps(e, old, values, nil)
				got := ShapeProps(e, nil)
				if !bytes.Equal(got[c.key], compact(c.value)) {
					t.Fatalf("%s %s: %s set to %s reads %s in %s", file, name, c.key, c.value, got[c.key], e.Bytes())
				}
				for key := range values {
					if key != c.key && !bytes.Equal(got[key], old[key]) {
						t.Fatalf("%s: setting %s changed %s to %s", e.Bytes(), c.key, key, got[key])
					}
				}
				if order := childOrder(e); order != "" {
					t.Fatalf("%s: %s", e.Bytes(), order)
				}
				old = got
			}
			values := maps.Clone(old)
			delete(values, "fill")
			delete(values, "line")
			SetShapeProps(e, old, values, nil)
			if got := ShapeProps(e, nil); got["fill"] != nil || got["line"] != nil {
				t.Fatalf("%s: fill and line not removed", e.Bytes())
			}
		})
	})
	t.Logf("%d spPr", n)
}

func compact(s string) json.RawMessage {
	var b bytes.Buffer
	json.Compact(&b, []byte(s))
	return b.Bytes()
}

// childOrder tells what is out of the order spPr imposes, "" if nothing.
func childOrder(e *xmldom.Element) string {
	rank := map[string]int{}
	for i, n := range spPrOrder {
		rank[n] = i
	}
	last := -1
	for _, c := range e.Elements() {
		r, ok := rank[c.Local]
		if !ok {
			continue
		}
		if r <= last {
			return c.Local + " out of order"
		}
		last = r
	}
	return ""
}
