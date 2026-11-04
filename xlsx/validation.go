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

// maxRules bounds the conditional formats and the validations of a sheet.
const maxRules = 1000

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
	text := func(e *xmldom.Element) string {
		if e == nil {
			return ""
		}
		return strings.TrimSpace(e.Text())
	}
	for _, v := range elements(root.Child(mainNS, "dataValidations"), "dataValidation") {
		out = appendValidation(out, v, v.Get("sqref"), text(v.Child(mainNS, "formula1")), text(v.Child(mainNS, "formula2")))
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
			f := func(local string) string {
				if f := v.Child(x14NS, local); f != nil {
					return text(f.Child(xmNS, "f"))
				}
				return ""
			}
			out = appendValidation(out, v, text(v.Child(xmNS, "sqref")), f("formula1"), f("formula2"))
		}
	}
	return out
}

func appendValidation(out []Validation, v *xmldom.Element, ref, f1, f2 string) []Validation {
	typ := v.Get("type")
	if typ == "none" {
		typ = ""
	}
	x := Validation{Ref: strings.Join(strings.Fields(ref), " "), Type: typ, NoArrow: truthy(v.Get("showDropDown")), Blank: truthy(v.Get("allowBlank"))}
	if x.Ref == "" || typ != "" && !slices.Contains(validTypes, typ) {
		return out
	}
	if typ != "" {
		if x.F1 = f1; f1 == "" {
			return out
		}
		if !slices.Contains(validNoCompare, typ) {
			x.Op = v.Get("operator")
			if x.Op == "between" {
				x.Op = ""
			}
			if x.Op != "" && !slices.Contains(validOps, x.Op) {
				return out
			}
			if x.Op == "" || x.Op == "notBetween" {
				if x.F2 = f2; f2 == "" {
					return out
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
		return out
	}
	return append(out, x)
}

// checkValidations tells whether validations are those the editor may set.
func checkValidations(raw json.RawMessage) bool {
	var list []Validation
	if json.Unmarshal(raw, &list) != nil || len(list) > maxRules {
		return false
	}
	for _, v := range list {
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
			if v.F1 != "" || v.F2 != "" || v.Op != "" || v.Err == "" && v.PromptTitle == "" && v.Prompt == "" {
				return false
			}
			continue
		}
		if !checkFormula(v.F1) || v.F2 != "" && !checkFormula(v.F2) {
			return false
		}
		if slices.Contains(validNoCompare, v.Type) {
			if v.Op != "" || v.F2 != "" {
				return false
			}
			if v.Type == "list" && strings.HasPrefix(v.F1, `"`) && utf16Len(v.F1) > maxList+2 {
				return false
			}
			continue
		}
		if between := v.Op == "" || v.Op == "notBetween"; between != (v.F2 != "") {
			return false
		}
	}
	return true
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
// of those it had, which Excel 2010 may keep in an extension.
func writeValidations(root *xmldom.Element, raw json.RawMessage) {
	var list []Validation
	_ = json.Unmarshal(raw, &list)
	if e := root.Child(mainNS, "dataValidations"); e != nil {
		root.Remove(e)
	}
	editExt(root, func(ext *xmldom.Element) {
		if all := ext.Child(x14NS, "dataValidations"); all != nil {
			ext.Remove(all)
		}
	})
	if len(list) == 0 {
		return
	}
	all := child(root, "dataValidations", worksheetOrder)
	all.Set("count", strconv.Itoa(len(list)))
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
