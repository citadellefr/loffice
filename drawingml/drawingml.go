// Package drawingml reads and writes the DrawingML shared by Word, Excel and
// PowerPoint: colors, fills, lines, geometries, positions and text bodies.
//
// Each property the editors show becomes a JSON value, which goes back into
// XML only when it changed: an element is patched, never rebuilt, so what
// the model leaves out stays as it was. The JSON forms are those the Dart
// package reads:
//
//	color     {"rgb":"FF0000"} {"scheme":"accent1"} {"sys":"windowText","rgb":"000000"}
//	          {"prst":"red"} {"scrgb":[100000,0,0]} {"hsl":[0,100000,50000]},
//	          with "mods":[["lumMod",75000],["alpha",50000],["gray"]] when transformed
//	fill      {"none":true} {"solid":color} {"grp":true}
//	          {"grad":{"stops":[[0,color],[100000,color]],"lin":5400000,"scaled":true}}
//	          {"patt":{"prst":"pct5","fg":color,"bg":color}}
//	          {"blip":{"media":"…","rect":[l,t,r,b],"tile":true}}
//	line      {"w":12700,"fill":fill,"dash":"dash","cap":"rnd","join":"round",
//	          "head":{"type":"triangle","w":"med","len":"med"},"tail":{…}}
//	geometry  {"prst":"roundRect","av":{"adj":"val 16667"}}
//	          {"cust":{"av":[…],"gd":[["g1","*/ w 1 2"]],"rect":["l","t","r","b"],
//	          "paths":[{"w":100,"h":100,"fill":"none","stroke":false,
//	          "d":[["M","0","0"],["L","w","h"],["A","wd2","hd2","0","cd4"],["Z"]]}]}}
//	xfrm      {"x":0,"y":0,"w":914400,"h":914400,"rot":5400000,"flipH":true,
//	          "cx":0,"cy":0,"cw":914400,"ch":914400}
//
// Text is a flow (package ot) whose attributes are strings: those of the
// runs on characters, those of the paragraphs on their marks, see Flow.
package drawingml

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

const (
	NS    = "http://schemas.openxmlformats.org/drawingml/2006/main"
	RelNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// newA is an element of the DrawingML namespace, prefixed "a" as Office
// writes it.
func newA(local string, attrs ...string) *xmldom.Element {
	return xmldom.New(NS, "a:"+local, attrs...)
}

// child is the first child of e in the DrawingML namespace named local.
func child(e *xmldom.Element, local string) *xmldom.Element {
	if e == nil {
		return nil
	}
	return e.Child(NS, local)
}

// children are the children of e in the DrawingML namespace.
func children(e *xmldom.Element) []*xmldom.Element {
	var out []*xmldom.Element
	if e == nil {
		return out
	}
	for _, c := range e.Elements() {
		if c.Space == NS {
			out = append(out, c)
		}
	}
	return out
}

func number(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

func flag(s string) bool {
	return s == "1" || s == "true" || s == "on"
}

// same tells whether two values would be written the same.
func same(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}

// setChild makes one child of e among the names in group, or none: it
// replaces the one there, if any, keeping the position schemas impose.
func setChild(e *xmldom.Element, group []string, c *xmldom.Element, order []string) {
	var old *xmldom.Element
	for _, g := range group {
		if old = child(e, g); old != nil {
			break
		}
	}
	switch {
	case c == nil && old != nil:
		e.Remove(old)
	case c != nil && old != nil && old.Local == c.Local:
		e.Replace(old, c)
	case c != nil:
		if old != nil {
			e.Remove(old)
		}
		e.Insert(c, order)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
