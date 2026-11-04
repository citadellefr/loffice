package xlsx

import (
	"encoding/json"
	"strings"
	"testing"
)

// rule is one conditional format, whose fields the later ones of the text
// override.
func rule(fields string) json.RawMessage {
	return json.RawMessage(`[{"ref":"A1:A5","pri":1,"type":"formula","f":["A1>1"],"style":{"font":{"b":true}},` + strings.TrimSuffix(fields, ",") + `}]`)
}

func TestConditionalChecked(t *testing.T) {
	scale := `"type":"colorScale","f":null,"style":null,`
	bar := `"type":"dataBar","f":null,"style":null,"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"638EC6"}],"minLen":10,"maxLen":90,`
	icons := `"type":"iconSet","f":null,"style":null,"icons":"4Rating","stops":[{"t":"percent","v":"0"},{"t":"percent","v":"25"},` +
		`{"t":"percent","v":"50"},{"t":"percent","v":"75"}],`
	for _, ok := range []string{
		`"ref":"B2:B5 D3 F:F 3:4"`, `"stop":true`, `"type":"top10","f":null,"rank":10`, `"type":"top10","f":null,"rank":1000,"bottom":true`,
		`"type":"aboveAverage","f":null,"stdDev":3,"below":true`, `"type":"duplicateValues","f":null`, `"type":"uniqueValues","f":null`,
		scale + `"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},{"theme":4,"tint":0.4}]`,
		scale + `"stops":[{"t":"min"},{"t":"percentile","v":"50"},{"t":"formula","v":"B1+1"}],"colors":[{"rgb":"F8696B"},{"auto":true},{"rgb":"63BE7B"}]`,
		bar, bar + `"maxLen":100,"minLen":0,"noValue":true,"stops":[{"t":"num","v":"0"},{"t":"max","gt":true}]`, icons,
		icons + `"icons":"3Arrows","stops":[{"t":"percent","v":"0"},{"t":"percent","v":"33"},{"t":"percent","v":"67"}],"reverse":true`,
	} {
		if !checkConditionals(rule(ok)) {
			t.Errorf("%s refused", ok)
		}
	}
	for _, bad := range []string{
		`"ref":""`, `"ref":"nope"`, `"ref":"A1:"`, `"ref":"A1,B2"`, `"type":"foo"`, `"type":"cellIs"`, `"type":""`,
		`"f":[]`, `"f":null`, `"f":["A1>1","A1<9"]`, `"f":[""]`, `"f":["SUM(("]`, `"f":["` + strings.Repeat("1+", maxFormula) + `1"]`,
		`"style":null`, `"style":{"fill":{"pattern":"bogus"}}`, `"style":{"font":{"color":{"rgb":"XYZ"}}}`, `"style":{"fmt":"` + strings.Repeat("0", 256) + `"}`,
		`"pri":0`, `"pri":-1`,
		`"type":"top10","f":null,"rank":0`, `"type":"top10","f":null,"rank":1001`, `"type":"top10","f":null,"rank":10,"style":null`,
		`"type":"aboveAverage","f":null,"stdDev":4`, `"type":"aboveAverage","f":null,"stdDev":-1`, `"type":"aboveAverage","f":null,"style":null`,
		`"type":"duplicateValues","f":null,"style":null`, `"type":"uniqueValues","f":null,"style":null`, `"type":"duplicateValues","f":["SUM(("]`,
		scale + `"stops":[{"t":"min"}],"colors":[{"rgb":"F8696B"}]`, scale,
		scale + `"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"}]`,
		scale + `"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},{"rgb":"63BE7B"},{"rgb":"000000"}]`,
		scale + `"stops":[{"t":"min"},{"t":"max"},{"t":"max"},{"t":"max"}],"colors":[{"auto":true},{"auto":true},{"auto":true},{"auto":true}]`,
		scale + `"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},{"rgb":"zz"}]`,
		scale + `"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},null]`,
		scale + `"stops":[{"t":"min"},{"t":"top"}],"colors":[{"auto":true},{"auto":true}]`,
		scale + `"stops":[{"t":"min"},{"t":"num"}],"colors":[{"auto":true},{"auto":true}]`,
		scale + `"stops":[{"t":"min"},{"t":"num","v":"(("}],"colors":[{"auto":true},{"auto":true}]`,
		bar + `"stops":[{"t":"min"}]`, bar + `"stops":[{"t":"min"},{"t":"max"},{"t":"max"}]`, bar + `"colors":[]`, bar + `"colors":[{"rgb":"?"}]`,
		bar + `"minLen":50,"maxLen":40`, bar + `"minLen":-1`, bar + `"maxLen":101`,
		icons + `"icons":""`, icons + `"icons":"4Rating2"`, icons + `"icons":"3Arrows"`, icons + `"stops":[{"t":"percent","v":"0"}]`,
		icons + `"stops":null`,
	} {
		if checkConditionals(rule(bad)) {
			t.Errorf("%s taken", bad)
		}
	}
	many := "[" + strings.Repeat(`{"ref":"A1","pri":1,"type":"duplicateValues","style":{}},`, maxRules) + `{"ref":"A1","pri":1,"type":"duplicateValues","style":{}}]`
	if checkConditionals(json.RawMessage(many)) {
		t.Error("too many conditional formats taken")
	}
	if checkConditionals(json.RawMessage(`{"ref":"A1"}`)) {
		t.Error("an object taken")
	}
}
