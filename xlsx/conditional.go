package xlsx

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const conditionalKey = "cf"

// A Conditional is a conditional format: the cells of Ref take a look when
// its rule holds. The file keeps it as XML: the sheet's "cf" show it and
// move with it, and the server calculates the looks it gives.
type Conditional struct {
	Ref string `json:"ref"`
	// Type is the rule: formula for those a formula decides, cellIs
	// written as one; top10, aboveAverage, duplicateValues,
	// uniqueValues, colorScale, dataBar or iconSet.
	Type string `json:"type"`
	// Pri orders the rules, the first 1; Stop leaves the next ones out
	// of the cells the rule holds for.
	Pri  int  `json:"pri"`
	Stop bool `json:"stop,omitempty"`
	// F are the formulas, written for the first cell of Ref.
	F     []string `json:"f,omitempty"`
	Style *Style   `json:"style,omitempty"`

	Rank    int  `json:"rank,omitempty"`
	Percent bool `json:"percent,omitempty"`
	Bottom  bool `json:"bottom,omitempty"`

	Below  bool `json:"below,omitempty"`
	Equal  bool `json:"equal,omitempty"`
	StdDev int  `json:"stdDev,omitempty"`

	// Stops are the thresholds of a scale, bar or set of icons, Colors
	// those of a scale or bar.
	Stops   []Threshold `json:"stops,omitempty"`
	Colors  []*Color    `json:"colors,omitempty"`
	Icons   string      `json:"icons,omitempty"`
	Reverse bool        `json:"reverse,omitempty"`
	NoValue bool        `json:"noValue,omitempty"`
	// MinLen and MaxLen bound the length of a bar, in percent of its cell.
	MinLen int `json:"minLen,omitempty"`
	MaxLen int `json:"maxLen,omitempty"`
}

// A Threshold is where a scale, bar or set of icons changes: the lowest
// or highest value, a number, a percent or percentile of the values, or
// what a formula gives.
type Threshold struct {
	Type string `json:"t"`
	Val  string `json:"v,omitempty"`
	// Above is a threshold the value must exceed, not only reach.
	Above bool `json:"gt,omitempty"`
}

// A cfRule is a conditional format with the element it was read from.
type cfRule struct {
	Conditional
	elem *xmldom.Element
}

// conditionals are the conditional formats of a worksheet, by priority.
func (d *Document) conditionals(root *xmldom.Element) []Conditional {
	var out []Conditional
	for _, r := range d.cfRules(root) {
		out = append(out, r.Conditional)
	}
	slices.SortStableFunc(out, func(a, b Conditional) int { return a.Pri - b.Pri })
	return out
}

// cfRules are the rules of the conditional formats of a worksheet that the
// model holds, in the order of the file.
func (d *Document) cfRules(root *xmldom.Element) []cfRule {
	var out []cfRule
	for _, cf := range elements(root, "conditionalFormatting") {
		ref, anchor, ok := cfArea(cf)
		if !ok {
			continue
		}
		for _, rule := range elements(cf, "cfRule") {
			if c, ok := d.conditional(rule, ref, anchor); ok {
				out = append(out, cfRule{c, rule})
			}
		}
	}
	return out
}

// cfArea is the areas of a conditional format, and the first cell, which
// its formulas are written for.
func cfArea(cf *xmldom.Element) (ref, anchor string, ok bool) {
	ref = strings.Join(strings.Fields(cf.Get("sqref")), " ")
	first, ok := formula.ParseArea(strings.SplitN(ref, " ", 2)[0])
	return ref, formula.CellName(first.R1, first.C1), ok
}

func (d *Document) conditional(rule *xmldom.Element, ref, anchor string) (Conditional, bool) {
	c := Conditional{Ref: ref, Type: rule.Get("type"), Pri: atoi(rule.Get("priority")), Stop: truthy(rule.Get("stopIfTrue"))}
	for _, f := range elements(rule, "formula") {
		c.F = append(c.F, strings.TrimSpace(f.Text()))
	}
	if id := rule.Get("dxfId"); id != "" {
		c.Style = d.styles.dxf(atoi(id))
	}
	text := `"` + strings.ReplaceAll(rule.Get("text"), `"`, `""`) + `"`
	switch c.Type {
	case "expression":
		c.Type = "formula"
	case "cellIs":
		f, ok := CellIsFormula(rule.Get("operator"), anchor, c.F)
		if !ok {
			return c, false
		}
		c.Type, c.F = "formula", []string{f}
	case "containsText", "notContainsText", "beginsWith", "endsWith", "containsBlanks", "notContainsBlanks",
		"containsErrors", "notContainsErrors", "timePeriod":
		if len(c.F) == 0 {
			f, ok := map[string]string{
				"containsText":      "NOT(ISERROR(SEARCH(" + text + "," + anchor + ")))",
				"notContainsText":   "ISERROR(SEARCH(" + text + "," + anchor + "))",
				"beginsWith":        "LEFT(" + anchor + ",LEN(" + text + "))=" + text,
				"endsWith":          "RIGHT(" + anchor + ",LEN(" + text + "))=" + text,
				"containsBlanks":    "LEN(TRIM(" + anchor + "))=0",
				"notContainsBlanks": "LEN(TRIM(" + anchor + "))>0",
				"containsErrors":    "ISERROR(" + anchor + ")",
				"notContainsErrors": "NOT(ISERROR(" + anchor + "))",
			}[c.Type]
			if !ok {
				return c, false
			}
			c.F = []string{f}
		}
		c.Type = "formula"
	case "top10":
		c.Rank, c.Percent, c.Bottom = atoi(rule.Get("rank")), truthy(rule.Get("percent")), truthy(rule.Get("bottom"))
	case "aboveAverage":
		above, set := rule.Attr("aboveAverage")
		c.Below = set && !truthy(above)
		c.Equal, c.StdDev = truthy(rule.Get("equalAverage")), atoi(rule.Get("stdDev"))
	case "duplicateValues", "uniqueValues":
	case "colorScale", "dataBar", "iconSet":
		scale := rule.Child(mainNS, c.Type)
		if scale == nil {
			return c, false
		}
		for _, v := range elements(scale, "cfvo") {
			gte, set := v.Attr("gte")
			c.Stops = append(c.Stops, Threshold{Type: v.Get("type"), Val: v.Get("val"), Above: set && !truthy(gte)})
		}
		for _, col := range elements(scale, "color") {
			c.Colors = append(c.Colors, d.styles.color(col))
		}
		c.Reverse = truthy(scale.Get("reverse"))
		if v, set := scale.Attr("showValue"); set && !truthy(v) {
			c.NoValue = true
		}
		switch c.Type {
		case "colorScale":
			if len(c.Stops) < 2 || len(c.Colors) != len(c.Stops) {
				return c, false
			}
		case "dataBar":
			if len(c.Stops) != 2 || len(c.Colors) == 0 {
				return c, false
			}
			c.MinLen, c.MaxLen = 10, 90
			if v, set := scale.Attr("minLength"); set {
				c.MinLen = atoi(v)
			}
			if v, set := scale.Attr("maxLength"); set {
				c.MaxLen = atoi(v)
			}
		case "iconSet":
			c.Icons = scale.Get("iconSet")
			if c.Icons == "" {
				c.Icons = "3TrafficLights1"
			}
			if len(c.Stops) < 3 {
				return c, false
			}
		}
	default:
		return c, false
	}
	if c.Type == "formula" && c.Style == nil {
		return c, false
	}
	return c, true
}

// CellIsFormula is the formula that holds when the cell at anchor compares
// with the formulas f as the operator of a cellIs rule says.
func CellIsFormula(op, anchor string, f []string) (string, bool) {
	switch {
	case (op == "between" || op == "notBetween") && len(f) == 2:
		between := "AND(" + anchor + ">=(" + f[0] + ")," + anchor + "<=(" + f[1] + "))"
		if op == "notBetween" {
			between = "NOT(" + between + ")"
		}
		return between, true
	case cellOperators[op] != "" && len(f) > 0:
		return anchor + cellOperators[op] + "(" + f[0] + ")", true
	}
	return "", false
}

// cellOperators are the operators of cellIs rules, as formulas write
// them.
var cellOperators = map[string]string{
	"equal": "=", "notEqual": "<>", "greaterThan": ">", "lessThan": "<", "greaterThanOrEqual": ">=",
	"lessThanOrEqual": "<=",
}

// dxf is the model of a differential format, what a conditional format
// changes of its cells. A solid fill takes its background color, as
// Excel paints them.
func (s *styles) dxf(i int) *Style {
	if i < 0 || i >= len(s.dxfs) {
		return nil
	}
	e := s.dxfs[i]
	st := &Style{}
	if f := e.Child(mainNS, "font"); f != nil {
		st.Font = s.font(f)
	}
	if f := e.Child(mainNS, "numFmt"); f != nil {
		st.Format = f.Get("formatCode")
	}
	if f := e.Child(mainNS, "fill"); f != nil {
		if p := f.Child(mainNS, "patternFill"); p != nil && p.Get("patternType") != "none" {
			fill := &Fill{Pattern: "solid"}
			for _, local := range []string{"fgColor", "bgColor"} {
				if c := p.Child(mainNS, local); c != nil {
					fill.Fg = s.color(c)
				}
			}
			if fill.Fg != nil {
				st.Fill = fill
			}
		}
	}
	if b := e.Child(mainNS, "border"); b != nil {
		st.Border = s.border(b)
	}
	return st
}

var (
	thresholdTypes = []string{"min", "max", "num", "percent", "percentile", "formula"}
	// iconSets are the sets of icons Excel has, with how many icons each
	// shows.
	iconSets = map[string]int{
		"3Arrows": 3, "3ArrowsGray": 3, "3Flags": 3, "3TrafficLights1": 3, "3TrafficLights2": 3, "3Signs": 3,
		"3Symbols": 3, "3Symbols2": 3, "4Arrows": 4, "4ArrowsGray": 4, "4RedToBlack": 4, "4Rating": 4,
		"4TrafficLights": 4, "5Arrows": 5, "5ArrowsGray": 5, "5Rating": 5, "5Quarters": 5,
	}
)

// checkConditionals tells whether conditional formats are those the editor
// may set.
func checkConditionals(raw json.RawMessage) bool {
	var rules []Conditional
	if json.Unmarshal(raw, &rules) != nil || len(rules) > MaxRules {
		return false
	}
	for _, c := range rules {
		if !c.Valid() {
			return false
		}
	}
	return true
}

// Valid tells whether the conditional format is one the editor may set.
func (c Conditional) Valid() bool {
	if !checkAreas(c.Ref) || c.Pri < 1 || c.Style != nil && !c.Style.valid() {
		return false
	}
	for _, f := range c.F {
		if !checkFormula(f) {
			return false
		}
	}
	for _, t := range c.Stops {
		if !slices.Contains(thresholdTypes, t.Type) || t.Val != "" && !checkFormula(t.Val) ||
			t.Val == "" && t.Type != "min" && t.Type != "max" {
			return false
		}
	}
	for _, col := range c.Colors {
		if col == nil || !validColor(col) {
			return false
		}
	}
	return checkRule(c)
}

// checkRule tells whether the rule of a conditional format is complete.
func checkRule(c Conditional) bool {
	switch c.Type {
	case "formula":
		return len(c.F) == 1 && c.Style != nil
	case "top10":
		return c.Style != nil && c.Rank >= 1 && c.Rank <= 1000
	case "aboveAverage":
		return c.Style != nil && c.StdDev >= 0 && c.StdDev <= 3
	case "duplicateValues", "uniqueValues":
		return c.Style != nil
	case "colorScale":
		return len(c.Stops) >= 2 && len(c.Stops) <= 3 && len(c.Colors) == len(c.Stops)
	case "dataBar":
		return len(c.Stops) == 2 && len(c.Colors) >= 1 && c.MinLen >= 0 && c.MinLen <= c.MaxLen && c.MaxLen <= 100
	case "iconSet":
		n, ok := iconSets[c.Icons]
		return ok && len(c.Stops) == n
	}
	return false
}
