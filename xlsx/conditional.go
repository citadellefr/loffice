package xlsx

import (
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

// conditionals are the conditional formats of a worksheet, by priority.
func (d *Document) conditionals(root *xmldom.Element) []Conditional {
	var out []Conditional
	for _, cf := range elements(root, "conditionalFormatting") {
		ref := strings.Join(strings.Fields(cf.Get("sqref")), " ")
		first, ok := formula.ParseArea(strings.SplitN(ref, " ", 2)[0])
		if !ok {
			continue
		}
		anchor := formula.CellName(first.R1, first.C1)
		for _, rule := range elements(cf, "cfRule") {
			if c, ok := d.conditional(rule, ref, anchor); ok {
				out = append(out, c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Conditional) int { return a.Pri - b.Pri })
	return out
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
		op := rule.Get("operator")
		var f string
		switch {
		case (op == "between" || op == "notBetween") && len(c.F) == 2:
			f = "AND(" + anchor + ">=(" + c.F[0] + ")," + anchor + "<=(" + c.F[1] + "))"
			if op == "notBetween" {
				f = "NOT(" + f + ")"
			}
		case cellOperators[op] != "" && len(c.F) > 0:
			f = anchor + cellOperators[op] + "(" + c.F[0] + ")"
		default:
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
