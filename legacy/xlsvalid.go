package legacy

import (
	"strings"

	"github.com/citadellefr/loffice/xlsx"
)

// The flags of a DV that tell what it holds.
const (
	dvType       = 0x0000000F
	dvAlert      = 0x00000070
	dvList       = 0x00000080
	dvBlank      = 0x00000100
	dvNoArrow    = 0x00000200
	dvShowPrompt = 0x00040000
	dvShowAlert  = 0x00080000
	dvOperator   = 0x00F00000
)

// dvTypes are what the cells take, as the rules of an .xlsx name them, and
// dvOperators how they compare; dvAlerts what is done of a wrong value.
var (
	dvTypes     = [...]string{"", "whole", "decimal", "list", "date", "time", "textLength", "custom"}
	dvOperators = [...]string{"", "notBetween", "equal", "notEqual", "greaterThan", "lessThan", "greaterThanOrEqual",
		"lessThanOrEqual"}
	dvAlerts = [...]string{"stop", "warning", "information"}
)

// validation reads the records of the data validations of a sheet: DVal,
// whose header holds nothing a conversion needs, and DV.
func (s *sheet) validation(id uint16, d []byte) {
	if id == recValidation && !s.dataValidation(d) {
		s.b.lose(Validation)
	}
}

// dataValidation reads a DV: what the cells take, the messages it shows,
// the formulas and the areas.
func (s *sheet) dataValidation(d []byte) bool {
	if len(d) < 4 || len(s.validations) >= xlsx.MaxRules {
		return false
	}
	flags := le.Uint32(d)
	c := &cursor{b: d[4:]}
	var texts [4]string
	for i := range texts {
		text, size, _ := longText(c.b)
		if size == 0 {
			return false
		}
		c.take(size)
		if text != "\x00" {
			texts[i] = text
		}
	}
	var codes [2][]byte
	for i := range codes {
		size := c.uint16()
		c.take(2)
		codes[i] = c.take(size)
	}
	count := c.uint16()
	ref, first, ok := sqref(c.b, count)
	if c.failed || !ok || int(flags&dvType) >= len(dvTypes) {
		return false
	}
	v := xlsx.Validation{
		Ref:     ref,
		Type:    dvTypes[flags&dvType],
		Op:      dvOperators[flags&dvOperator>>20&7],
		NoArrow: flags&dvNoArrow != 0,
		Blank:   flags&dvBlank != 0,
	}
	if flags&dvShowAlert != 0 {
		v.Err, v.ErrTitle, v.ErrText = dvAlerts[min(flags&dvAlert>>4, 2)], texts[1], texts[3]
	}
	if flags&dvShowPrompt != 0 {
		v.PromptTitle, v.Prompt = texts[0], texts[2]
	}
	if v.Type != "" {
		o := origin{row: first[0], col: first[2], rows: biffRows, cols: biffCols}
		var ok bool
		if v.F1, ok = s.dvFormula(codes[0], flags&dvList != 0, o); !ok {
			return false
		}
		switch {
		case v.Type == "list" || v.Type == "custom":
			v.Op = ""
		case v.Op == "" || v.Op == "notBetween":
			if v.F2, ok = s.dvFormula(codes[1], false, o); !ok {
				return false
			}
		}
	}
	if !v.Valid() {
		return v.Type == "" && v.Err == "" && v.Prompt == "" && v.PromptTitle == ""
	}
	s.validations = append(s.validations, v)
	return true
}

// dvFormula reads a formula of a validation: a list written out in the
// record is a string whose values end at a null.
func (s *sheet) dvFormula(code []byte, list bool, o origin) (string, bool) {
	if len(code) == 0 {
		return "", false
	}
	if list {
		items, size := shortText(code[1:])
		if code[0] != ptgStr || size == 0 || size+1 != len(code) || strings.ContainsAny(items, `,"`) {
			return "", false
		}
		return `"` + strings.Join(strings.Split(strings.TrimRight(items, "\x00"), "\x00"), ",") + `"`, true
	}
	text, err := s.b.formula(code, nil, o)
	if err != nil {
		s.b.loseFormula(err)
		return "", false
	}
	return text, true
}
