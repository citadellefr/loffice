package xlsx

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const (
	x14NS    = "http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"
	xmNS     = "http://schemas.microsoft.com/office/excel/2006/main"
	validKey = "valid"
)

// A Validation restricts what the cells of Ref take, and says so when they
// are selected or given a wrong value. The file keeps it as XML: the
// sheet's "valid" show it and move with it.
type Validation struct {
	// Ref are the areas the validation covers, as sqref writes them.
	Ref string `json:"ref"`
	// Type is what the cells take: list, whole, decimal, date, time,
	// textLength or custom; any value when it is empty, so that only the
	// messages count.
	Type string `json:"type,omitempty"`
	// Op compares the value with F1, and F2 when it lies between: it is
	// between when empty, or notBetween, equal, notEqual, greaterThan,
	// lessThan, greaterThanOrEqual or lessThanOrEqual. A list and a custom
	// rule have none.
	Op string `json:"op,omitempty"`
	// F1 and F2 are formulas, written for the first cell of Ref. A list is
	// values in quotes separated by commas, the cells or the name that
	// holds them, which the editor offers in a drop-down.
	F1      string `json:"f1,omitempty"`
	F2      string `json:"f2,omitempty"`
	NoArrow bool   `json:"noArrow,omitempty"`
	Blank   bool   `json:"blank,omitempty"`
	// Err is how a value that is not taken is met: "stop", "warning" or
	// "information"; it is when it is empty.
	Err         string `json:"err,omitempty"`
	ErrTitle    string `json:"errTitle,omitempty"`
	ErrText     string `json:"errText,omitempty"`
	PromptTitle string `json:"promptTitle,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
}

var (
	validTypes     = []string{"list", "whole", "decimal", "date", "time", "textLength", "custom"}
	validOps       = []string{"notBetween", "equal", "notEqual", "greaterThan", "lessThan", "greaterThanOrEqual", "lessThanOrEqual"}
	validAlerts    = []string{"stop", "warning", "information"}
	validNoCompare = []string{"list", "custom"}
)

// MaxRules bounds the conditional formats and the validations of a sheet.
const MaxRules = 1000

// The limits of Excel on the messages of a validation, and on the values of
// a list written out.
const (
	maxTitle  = 32
	maxPrompt = 255
	maxList   = 255
)

// validations are the validations of a worksheet, those Excel 2010 writes
// in an extension for lists on other sheets included.
func validations(root *xmldom.Element) []Validation {
	var out []Validation
	eachValidation(root, func(_, _ *xmldom.Element, v Validation, ok bool) {
		if ok {
			out = append(out, v)
		}
	})
	return out
}

// eachValidation calls visit with each validation of a worksheet, in its
// list, and what the model holds of it: ok is false when it cannot show it.
func eachValidation(root *xmldom.Element, visit func(list, e *xmldom.Element, v Validation, ok bool)) {
	text := func(e *xmldom.Element) string {
		if e == nil {
			return ""
		}
		return strings.TrimSpace(e.Text())
	}
	if all := root.Child(mainNS, "dataValidations"); all != nil {
		for _, e := range elements(all, "dataValidation") {
			v, ok := readValidation(e, e.Get("sqref"), text(e.Child(mainNS, "formula1")), text(e.Child(mainNS, "formula2")))
			visit(all, e, v, ok)
		}
	}
	for _, ext := range elements(root.Child(mainNS, "extLst"), "ext") {
		all := ext.Child(x14NS, "dataValidations")
		if all == nil {
			continue
		}
		for _, e := range elementsIn(all, x14NS, "dataValidation") {
			f := func(local string) string {
				if f := e.Child(x14NS, local); f != nil {
					return text(f.Child(xmNS, "f"))
				}
				return ""
			}
			v, ok := readValidation(e, text(e.Child(xmNS, "sqref")), f("formula1"), f("formula2"))
			visit(all, e, v, ok)
		}
	}
}

func readValidation(v *xmldom.Element, ref, f1, f2 string) (Validation, bool) {
	typ := v.Get("type")
	if typ == "none" {
		typ = ""
	}
	x := Validation{Ref: strings.Join(strings.Fields(ref), " "), Type: typ, NoArrow: truthy(v.Get("showDropDown")), Blank: truthy(v.Get("allowBlank"))}
	if x.Ref == "" || typ != "" && !slices.Contains(validTypes, typ) {
		return x, false
	}
	if typ != "" {
		if x.F1 = f1; f1 == "" {
			return x, false
		}
		if !slices.Contains(validNoCompare, typ) {
			x.Op = v.Get("operator")
			if x.Op == "between" {
				x.Op = ""
			}
			if x.Op != "" && !slices.Contains(validOps, x.Op) {
				return x, false
			}
			if x.Op == "" || x.Op == "notBetween" {
				if x.F2 = f2; f2 == "" {
					return x, false
				}
			}
		}
	}
	if truthy(v.Get("showErrorMessage")) {
		x.Err = v.Get("errorStyle")
		if x.Err != "warning" && x.Err != "information" {
			x.Err = "stop"
		}
		x.ErrTitle, x.ErrText = v.Get("errorTitle"), v.Get("error")
	}
	if truthy(v.Get("showInputMessage")) {
		x.PromptTitle, x.Prompt = v.Get("promptTitle"), v.Get("prompt")
	}
	if typ == "" && x.Err == "" && x.PromptTitle == "" && x.Prompt == "" {
		return x, false
	}
	return x, true
}

// checkValidations tells whether validations are those the editor may set.
func checkValidations(raw json.RawMessage) bool {
	var list []Validation
	if json.Unmarshal(raw, &list) != nil || len(list) > MaxRules {
		return false
	}
	for _, v := range list {
		if !v.Valid() {
			return false
		}
	}
	return true
}

// Valid tells whether the validation is one the editor may set.
func (v Validation) Valid() bool {
	if !checkAreas(v.Ref) || v.Type != "" && !slices.Contains(validTypes, v.Type) ||
		v.Err != "" && !slices.Contains(validAlerts, v.Err) ||
		v.Op != "" && !slices.Contains(validOps, v.Op) ||
		utf16Len(v.ErrTitle) > maxTitle || utf16Len(v.PromptTitle) > maxTitle ||
		utf16Len(v.ErrText) > maxPrompt || utf16Len(v.Prompt) > maxPrompt {
		return false
	}
	if v.Err == "" && (v.ErrTitle != "" || v.ErrText != "") {
		return false
	}
	if v.Type == "" {
		return v.F1 == "" && v.F2 == "" && v.Op == "" && (v.Err != "" || v.PromptTitle != "" || v.Prompt != "")
	}
	if !checkFormula(v.F1) || v.F2 != "" && !checkFormula(v.F2) {
		return false
	}
	if slices.Contains(validNoCompare, v.Type) {
		if v.Op != "" || v.F2 != "" {
			return false
		}
		return v.Type != "list" || !strings.HasPrefix(v.F1, `"`) || utf16Len(v.F1) <= maxList+2
	}
	between := v.Op == "" || v.Op == "notBetween"
	return between == (v.F2 != "")
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// checkAreas tells whether ref is areas separated by spaces.
func checkAreas(ref string) bool {
	areas := strings.Fields(ref)
	for _, a := range areas {
		if _, ok := formula.ParseArea(a); !ok {
			return false
		}
	}
	return len(areas) > 0
}

// checkFormula tells whether text is a formula the workbook takes.
func checkFormula(text string) bool {
	if text == "" || len(text) > maxFormula {
		return false
	}
	_, err := formula.Parse(text)
	return err == nil
}

// writeValidations gives a worksheet the validations of its sheet, in place
// of those it read, which Excel 2010 may keep in an extension. The ones the
// model cannot show stay where they are.
func writeValidations(root *xmldom.Element, raw json.RawMessage) {
	var list []Validation
	_ = json.Unmarshal(raw, &list)
	eachValidation(root, func(all, e *xmldom.Element, _ Validation, ok bool) {
		if ok {
			all.Remove(e)
		}
	})
	editExt(root, func(ext *xmldom.Element) {
		if all := ext.Child(x14NS, "dataValidations"); all != nil {
			countValidations(ext, all)
		}
	})
	all := root.Child(mainNS, "dataValidations")
	if all != nil {
		countValidations(root, all)
	}
	if len(list) == 0 {
		return
	}
	all = child(root, "dataValidations", worksheetOrder)
	p := prefixOf(all)
	for _, v := range list {
		e := xmldom.New(mainNS, p+"dataValidation")
		setAttr(e, "type", v.Type)
		if v.Err == "warning" || v.Err == "information" {
			e.Set("errorStyle", v.Err)
		}
		setAttr(e, "operator", v.Op)
		setAttr(e, "allowBlank", boolAttr(v.Blank))
		setAttr(e, "showDropDown", boolAttr(v.NoArrow))
		if v.PromptTitle != "" || v.Prompt != "" {
			e.Set("showInputMessage", "1")
		}
		setAttr(e, "showErrorMessage", boolAttr(v.Err != ""))
		setAttr(e, "errorTitle", v.ErrTitle)
		setAttr(e, "error", v.ErrText)
		setAttr(e, "promptTitle", v.PromptTitle)
		setAttr(e, "prompt", v.Prompt)
		e.Set("sqref", v.Ref)
		for i, f := range []string{v.F1, v.F2} {
			if f != "" {
				x := xmldom.New(mainNS, p+"formula"+strconv.Itoa(i+1))
				x.Append(xmldom.EscapeText(f))
				e.Append(x)
			}
		}
		all.Append(e)
	}
	all.Set("count", strconv.Itoa(len(all.Elements())))
}

// countValidations counts the validations of a list, and removes it from
// its parent when it has none.
func countValidations(parent, all *xmldom.Element) {
	if n := len(all.Elements()); n == 0 {
		parent.Remove(all)
	} else {
		all.Set("count", strconv.Itoa(n))
	}
}

// editExt changes the extensions of a worksheet, and removes those it
// leaves empty.
func editExt(root *xmldom.Element, edit func(ext *xmldom.Element)) {
	list := root.Child(mainNS, "extLst")
	if list == nil {
		return
	}
	for _, ext := range elements(list, "ext") {
		n := len(ext.Elements())
		if edit(ext); n > 0 && len(ext.Elements()) == 0 {
			list.Remove(ext)
		}
	}
	if len(list.Elements()) == 0 {
		root.Remove(list)
	}
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
