package xlsx

import (
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const (
	x14NS    = "http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"
	xmNS     = "http://schemas.microsoft.com/office/excel/2006/main"
	listsKey = "lists"
)

// A List validates cells with the values of a list, which the editor
// offers in a drop-down. The file keeps it as XML: the sheet's "lists"
// show it and move with it.
type List struct {
	// Ref are the areas the list validates, as sqref writes them.
	Ref string `json:"ref"`
	// Src is the list as a formula: values in quotes separated by
	// commas, the cells or the name that holds them.
	Src     string `json:"src"`
	NoArrow bool   `json:"noArrow,omitempty"`
	Blank   bool   `json:"blank,omitempty"`
	// Err is how a value out of the list is taken: "stop", "warning" or
	// "information"; any value is when it is empty.
	Err         string `json:"err,omitempty"`
	ErrTitle    string `json:"errTitle,omitempty"`
	ErrText     string `json:"errText,omitempty"`
	PromptTitle string `json:"promptTitle,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
}

// validationLists are the list validations of a worksheet, those Excel 2010 writes
// in an extension for lists on other sheets included.
func validationLists(root *xmldom.Element) []List {
	var out []List
	for _, v := range elements(root.Child(mainNS, "dataValidations"), "dataValidation") {
		out = appendList(out, v, v.Get("sqref"), v.Child(mainNS, "formula1"))
	}
	for _, ext := range elements(root.Child(mainNS, "extLst"), "ext") {
		all := ext.Child(x14NS, "dataValidations")
		if all == nil {
			continue
		}
		for _, v := range all.Elements() {
			if v.Space != x14NS || v.Local != "dataValidation" {
				continue
			}
			var ref string
			if s := v.Child(xmNS, "sqref"); s != nil {
				ref = s.Text()
			}
			var f *xmldom.Element
			if f1 := v.Child(x14NS, "formula1"); f1 != nil {
				f = f1.Child(xmNS, "f")
			}
			out = appendList(out, v, ref, f)
		}
	}
	return out
}

func appendList(out []List, v *xmldom.Element, ref string, f *xmldom.Element) []List {
	ref = strings.Join(strings.Fields(ref), " ")
	if v.Get("type") != "list" || f == nil || ref == "" {
		return out
	}
	src := strings.TrimSpace(f.Text())
	if src == "" {
		return out
	}
	l := List{Ref: ref, Src: src, NoArrow: truthy(v.Get("showDropDown")), Blank: truthy(v.Get("allowBlank"))}
	if truthy(v.Get("showErrorMessage")) {
		l.Err = v.Get("errorStyle")
		if l.Err != "warning" && l.Err != "information" {
			l.Err = "stop"
		}
		l.ErrTitle, l.ErrText = v.Get("errorTitle"), v.Get("error")
	}
	if truthy(v.Get("showInputMessage")) {
		l.PromptTitle, l.Prompt = v.Get("promptTitle"), v.Get("prompt")
	}
	return append(out, l)
}

// shiftRef is a list of areas of sheet own, named sheet, once rows or
// columns are inserted or removed at at: those removed are left out.
func shiftRef(ref, sheet string, rows bool, at, n int) string {
	var out []string
	for _, a := range strings.Fields(ref) {
		g, err := formula.Shift(a, sheet, true, rows, at, n)
		if err != nil {
			g = a
		}
		if !strings.Contains(g, "#REF!") {
			out = append(out, g)
		}
	}
	return strings.Join(out, " ")
}
