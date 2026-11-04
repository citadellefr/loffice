package xlsx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	rulesCellIs = `<cfRule type="cellIs" dxfId="0" priority="1" operator="greaterThan"><formula>3</formula></cfRule>`
	rulesExpr   = `<cfRule type="expression" dxfId="0" priority="2"><formula>A1&lt;2</formula></cfRule>`
	rulesBar    = `<cfRule type="dataBar" priority="3"><dataBar><cfvo type="min"/><cfvo type="max"/><color rgb="FF638EC6"/></dataBar>` +
		`<extLst><ext uri="{B025F937-C7B1-47D3-B67F-A62EFF666E3E}" xmlns:x14="` + x14NS + `"><x14:id>{AAAA}</x14:id></ext></extLst></cfRule>`
	rulesText = `<cfRule type="containsText" dxfId="0" priority="4" operator="containsText" text="x"><formula>NOT(ISERROR(SEARCH("x",C1)))</formula></cfRule>`
)

// One conditional format of each type, and validations of each kind.
const (
	addedConditionals = `[
		{"ref":"E1:E9 G1","type":"formula","pri":4,"stop":true,"f":["AND(E1>=(1),E1<=(9))"],"style":{"fmt":"0.00 \"€\"",
			"font":{"b":true,"i":true,"color":{"rgb":"C00000"}},"fill":{"pattern":"solid","fg":{"rgb":"FFC7CE"}},
			"border":{"b":{"style":"thin","color":{"rgb":"000000"}},"t":{"style":"double"}}}},
		{"ref":"E1:E9","type":"top10","pri":5,"rank":3,"percent":true,"bottom":true,"style":{"font":{"b":true}}},
		{"ref":"F1:F9","type":"aboveAverage","pri":6,"below":true,"equal":true,"stdDev":2,"style":{"font":{"b":true}}},
		{"ref":"F1:F9","type":"duplicateValues","pri":7,"style":{"fill":{"pattern":"solid","fg":{"theme":4,"tint":0.4}}}},
		{"ref":"F1:F9","type":"uniqueValues","pri":8,"stop":true,"style":{"font":{"strike":true}}},
		{"ref":"H1:H9","type":"colorScale","pri":9,"stops":[{"t":"min"},{"t":"percentile","v":"50"},{"t":"max"}],
			"colors":[{"rgb":"F8696B"},{"rgb":"FFEB84"},{"rgb":"63BE7B"}]},
		{"ref":"H1:H9","type":"dataBar","pri":10,"stops":[{"t":"num","v":"0"},{"t":"formula","v":"MAX($A$1:$A$9)","gt":true}],
			"colors":[{"rgb":"FF0000"}],"minLen":5,"maxLen":95,"noValue":true},
		{"ref":"I1:I9","type":"iconSet","pri":11,"icons":"4Rating","reverse":true,"noValue":true,
			"stops":[{"t":"percent","v":"0"},{"t":"percent","v":"25"},{"t":"percent","v":"50"},{"t":"percent","v":"75","gt":true}]}
	]`
	writtenValidations = `[
		{"ref":"A1:A5 C1","type":"whole","op":"greaterThan","f1":"0","blank":true,"err":"stop","errTitle":"Non","errText":"Positif <&> \"x\"",
			"promptTitle":"Entier","prompt":"Un entier\nau moins"},
		{"ref":"B1","type":"decimal","f1":"0","f2":"1","err":"warning"},
		{"ref":"B2","type":"date","op":"notBetween","f1":"43831","f2":"DATE(2020,12,31)","err":"information"},
		{"ref":"B3","type":"time","op":"lessThan","f1":"0.5"},
		{"ref":"B4","type":"textLength","op":"lessThanOrEqual","f1":"10","err":"stop"},
		{"ref":"B5","type":"custom","f1":"ISNUMBER(B5)","err":"stop"},
		{"ref":"D1:D5","type":"list","f1":"Feuil2!$A$1:$A$3","blank":true,"noArrow":true,"err":"stop"},
		{"ref":"E1","type":"list","f1":"\"a,b,c\""},
		{"ref":"F1","promptTitle":"Aide","prompt":"Saisir"},
		{"ref":"G1","err":"warning","errText":"Attention"},
		{"ref":"H1","type":"whole","op":"notEqual","f1":"5"},
		{"ref":"H2","type":"whole","op":"equal","f1":"5"},
		{"ref":"H3","type":"whole","op":"greaterThanOrEqual","f1":"5"}
	]`
)

// rulesBook is a workbook whose sheet has conditional formats, one with an
// extension of Excel 2010, and validations, one in an extension too.
func rulesBook(t *testing.T) (*Document, *ot.Tree, []byte) {
	t.Helper()
	return rulesBookWith(t, "", "")
}

// rulesBookWith has the validations given in its two lists.
func rulesBookWith(t *testing.T, listed, extended string) (*Document, *ot.Tree, []byte) {
	t.Helper()
	const rels = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	data := zipOf(t, map[string][]byte{
		"[Content_Types].xml": []byte(xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="` + typeWorksheet + `"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="` + typeStyles + `"/></Types>`),
		"_rels/.rels": []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relOfficeDocument + `" Target="xl/workbook.xml"/></Relationships>`),
		"xl/workbook.xml": []byte(xmlHeader + `<workbook xmlns="` + mainNS + `" xmlns:r="` + rels + `">` +
			`<sheets><sheet name="Feuil1" sheetId="1" r:id="rId1"/></sheets></workbook>`),
		"xl/_rels/workbook.xml.rels": []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relWorksheet + `" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + relStyles + `" Target="styles.xml"/></Relationships>`),
		"xl/styles.xml": []byte(xmlHeader + `<styleSheet xmlns="` + mainNS + `">` +
			`<fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellXfs>` +
			`<dxfs count="1"><dxf><font><b/></font><fill><patternFill><bgColor rgb="FFFFC7CE"/></patternFill></fill></dxf></dxfs></styleSheet>`),
		"xl/worksheets/sheet1.xml": []byte(xmlHeader + `<worksheet xmlns="` + mainNS + `"><sheetData>` +
			`<row r="1"><c r="A1"><v>1</v></c><c r="B1"><v>2</v></c></row></sheetData>` +
			`<conditionalFormatting sqref="A1:A5">` + rulesCellIs + rulesExpr + `</conditionalFormatting>` +
			`<conditionalFormatting sqref="B1:B5">` + rulesBar + `</conditionalFormatting>` +
			`<conditionalFormatting sqref="C1:C5">` + rulesText + `</conditionalFormatting>` +
			`<dataValidations count="1"><dataValidation type="whole" operator="greaterThan" allowBlank="1" showErrorMessage="1" sqref="A1:A5">` +
			`<formula1>0</formula1></dataValidation>` + listed + `</dataValidations>` +
			`<extLst><ext uri="{78C0D931-6437-407d-A8EE-F0AAD7539E65}" xmlns:x14="` + x14NS + `" xmlns:xm="` + xmNS + `">` +
			`<x14:conditionalFormattings><x14:conditionalFormatting><x14:cfRule type="dataBar" id="{AAAA}"><x14:dataBar minLength="0" maxLength="100">` +
			`<x14:cfvo type="autoMin"/><x14:cfvo type="autoMax"/></x14:dataBar></x14:cfRule><xm:sqref>B1:B5</xm:sqref></x14:conditionalFormatting>` +
			`</x14:conditionalFormattings></ext>` +
			`<ext uri="{CCE6A557-97BC-4b89-ADB6-D9C93CAAB3DF}" xmlns:x14="` + x14NS + `" xmlns:xm="` + xmNS + `">` +
			`<x14:dataValidations count="1"><x14:dataValidation type="list" allowBlank="1" showInputMessage="1" promptTitle="Choix" prompt="Un nom">` +
			`<x14:formula1><xm:f>Feuil2!$A$1:$A$3</xm:f></x14:formula1><xm:sqref>D1:D5</xm:sqref></x14:dataValidation>` + extended + `</x14:dataValidations></ext>` +
			`</extLst></worksheet>`),
	})
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree, data
}

// saved writes the workbook, and returns what it reads as and the text of
// its sheet and of its styles.
func saved(t *testing.T, d *Document, tree *ot.Tree) (*Document, *ot.Tree, string, string, []byte) {
	t.Helper()
	data, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	again, read, err := Open(data, Limits{})
	if err != nil {
		t.Fatalf("saved workbook does not open: %v", err)
	}
	pkg, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	sheet, _ := pkg.Read("xl/worksheets/sheet1.xml")
	styles, _ := pkg.Read("xl/styles.xml")
	return again, read, string(sheet), string(styles), data
}

// setRules sets the conditional formats and validations of the sheet, as a
// client does.
func setRules(t *testing.T, d *Document, tree *ot.Tree, attrs ot.Values) {
	t.Helper()
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: attrs}})
}

// sameRules fails unless the attribute of the sheet reads as written.
func sameRules[T any](t *testing.T, tree *ot.Tree, key string, want []T) {
	t.Helper()
	var got []T
	_ = json.Unmarshal(tree.Node("S1").Attrs[key], &got)
	if g, w := mustJSON(got), mustJSON(want); string(g) != string(w) {
		t.Errorf("%s read as\n%s\nwritten as\n%s", key, g, w)
	}
}

func TestConditionalWrite(t *testing.T) {
	d, tree, _ := rulesBook(t)
	var rules []Conditional
	_ = json.Unmarshal(tree.Node("S1").Attrs[conditionalKey], &rules)
	if len(rules) != 4 || rules[0].F[0] != "A1>(3)" || rules[1].F[0] != "A1<2" || rules[2].Type != "dataBar" || rules[3].Type != "formula" {
		t.Fatalf("read as %s", tree.Node("S1").Attrs[conditionalKey])
	}

	// the containsText rule goes, one of each other kind comes
	var added []Conditional
	_ = json.Unmarshal([]byte(addedConditionals), &added)
	want := append(rules[:3:3], added...)
	setRules(t, d, tree, ot.Values{conditionalKey: mustJSON(want)})
	_, read, sheet, styles, _ := saved(t, d, tree)
	sameRules(t, read, conditionalKey, want)
	for _, kept := range []string{rulesCellIs, rulesExpr, rulesBar, `<x14:cfRule type="dataBar" id="{AAAA}">`} {
		if !strings.Contains(sheet, kept) {
			t.Errorf("no %s in %s", kept, sheet)
		}
	}
	if strings.Contains(sheet, "containsText") {
		t.Errorf("rule left in %s", sheet)
	}
	if !strings.Contains(styles, `<dxfs count="5">`) || !strings.Contains(styles, `<fill><patternFill><bgColor rgb="FFFFC7CE"/></patternFill></fill>`) {
		t.Errorf("styles written as %s", styles)
	}

	// rules renumbered, one moved to another area, the bar lost
	moved := rules[1]
	moved.Ref = "B1:B5"
	want = []Conditional{moved, rules[0], added[1]}
	want[0].Pri, want[1].Pri, want[2].Pri = 1, 2, 3
	d, tree, _, _, _ = saved(t, d, tree)
	d, tree = reopenDoc(t, d, tree)
	setRules(t, d, tree, ot.Values{conditionalKey: mustJSON(want)})
	d, read, sheet, _, _ = saved(t, d, tree)
	sameRules(t, read, conditionalKey, want)
	if strings.Contains(sheet, "x14:cfRule") || strings.Contains(sheet, "{AAAA}") || !strings.Contains(sheet, "<x14:dataValidations") {
		t.Errorf("extension of the lost bar left, or the other one lost, in %s", sheet)
	}

	// all gone
	setRules(t, d, tree, ot.Values{conditionalKey: json.RawMessage(`null`)})
	_, read, sheet, _, _ = saved(t, d, tree)
	if read.Node("S1").Attrs[conditionalKey] != nil || strings.Contains(sheet, "conditionalFormatting") || strings.Contains(sheet, "cfRule") {
		t.Errorf("conditional formats left in %s", sheet)
	}
}

// reopenDoc opens the workbook the document last saved again, to edit it.
func reopenDoc(t *testing.T, d *Document, tree *ot.Tree) (*Document, *ot.Tree) {
	t.Helper()
	data, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err = Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

func TestValidationWrite(t *testing.T) {
	d, tree, _ := rulesBook(t)
	var valid []Validation
	_ = json.Unmarshal(tree.Node("S1").Attrs[validKey], &valid)
	if len(valid) != 2 || valid[0].Type != "whole" || valid[0].Op != "greaterThan" || valid[1].F1 != "Feuil2!$A$1:$A$3" || valid[1].Prompt != "Un nom" {
		t.Fatalf("read as %s", tree.Node("S1").Attrs[validKey])
	}

	var written []Validation
	_ = json.Unmarshal([]byte(writtenValidations), &written)
	setRules(t, d, tree, ot.Values{validKey: mustJSON(written)})
	d, read, sheet, _, _ := saved(t, d, tree)
	sameRules(t, read, validKey, written)
	if !strings.Contains(sheet, `<dataValidations count="13">`) || strings.Contains(sheet, "x14:dataValidation") || strings.Contains(sheet, "extLst") &&
		!strings.Contains(sheet, "x14:cfRule") {
		t.Errorf("validations written as %s", sheet)
	}

	// one changed, the others as they were
	written = append(written[:1:1], Validation{Ref: "J1", Type: "list", F1: "$A$1:$A$3", Err: "stop"})
	setRules(t, d, read, ot.Values{validKey: mustJSON(written)})
	_, read, _, _, _ = saved(t, d, read)
	sameRules(t, read, validKey, written)

	setRules(t, d, read, ot.Values{validKey: json.RawMessage(`null`)})
	_, read, sheet, _, _ = saved(t, d, read)
	if read.Node("S1").Attrs[validKey] != nil || strings.Contains(sheet, "dataValidation") {
		t.Errorf("validations left in %s", sheet)
	}
}

const (
	keptListed   = `<dataValidation type="whole" sqref="F2:F4"><formula1>1</formula1></dataValidation>`
	keptExtended = `<x14:dataValidation type="whole"><x14:formula1><xm:f>1</xm:f></x14:formula1><xm:sqref>G2:G4</xm:sqref></x14:dataValidation>`
)

// The validations the model cannot show stay as they are when another is
// edited, move with the rows, and go with the cells they covered.
func TestValidationKept(t *testing.T) {
	d, tree, _ := rulesBookWith(t, keptListed, keptExtended)
	var valid []Validation
	_ = json.Unmarshal(tree.Node("S1").Attrs[validKey], &valid)
	if len(valid) != 2 {
		t.Fatalf("read as %s", tree.Node("S1").Attrs[validKey])
	}
	setRules(t, d, tree, ot.Values{validKey: mustJSON(append(valid[:1:1], Validation{Ref: "J1", Type: "list", F1: "$A$1:$A$3", Err: "stop"}))})
	d, tree, sheet, _, _ := saved(t, d, tree)
	for _, want := range []string{keptListed, keptExtended, `<dataValidations count="3">`, `<x14:dataValidations count="1">`, `sqref="J1"`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("no %s in %s", want, sheet)
		}
	}
	if strings.Contains(sheet, "Feuil2") {
		t.Errorf("validation edited left in %s", sheet)
	}

	if err := moveRows(t, d, tree, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 1, N: 2}}); err != nil {
		t.Fatal(err)
	}
	_, _, sheet, _, _ = saved(t, d, tree)
	for _, want := range []string{`sqref="F4:F6"`, `<xm:sqref>G4:G6</xm:sqref>`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("no %s in %s", want, sheet)
		}
	}

	if err := moveRows(t, d, tree, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 4, N: 3}}); err != nil {
		t.Fatal(err)
	}
	_, _, sheet, _, _ = saved(t, d, tree)
	if strings.Contains(sheet, "F4") || strings.Contains(sheet, "G4") || strings.Contains(sheet, "x14:dataValidation") || !strings.Contains(sheet, `<dataValidations count="2">`) {
		t.Errorf("validations of removed cells left in %s", sheet)
	}
}

// A workbook whose conditional formats and validations are not edited is
// written back as it was.
func TestRulesUnchanged(t *testing.T) {
	d, tree, data := rulesBook(t)
	if got, err := d.Save(tree); err != nil || string(got) != string(data) {
		t.Errorf("saved differently: %v", err)
	}
	_, _, sheet, _, _ := saved(t, d, tree)
	for _, kept := range []string{rulesCellIs, rulesExpr, rulesBar, rulesText} {
		if !strings.Contains(sheet, kept) {
			t.Errorf("no %s in %s", kept, sheet)
		}
	}
}

func TestRulesRefused(t *testing.T) {
	d, tree, _ := rulesBook(t)
	for _, e := range []ot.Values{
		{conditionalKey: json.RawMessage(`[{"ref":"A1","pri":1,"type":"formula","f":["A1>1"]}]`)},
		{validKey: json.RawMessage(`[{"ref":"A1","type":"whole"}]`)},
		{conditionalKey: json.RawMessage(`"x"`)},
	} {
		if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: e}}, ""); err != ErrReadOnly {
			t.Errorf("%v: got %v", e, err)
		}
	}
}
