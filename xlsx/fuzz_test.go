package xlsx

import (
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/opc"
)

const fuzzSheet = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetViews><sheetView workbookViewId="0"><pane xSplit="1" ySplit="1" topLeftCell="B2" state="frozen"/></sheetView></sheetViews>
<cols><col min="1" max="2" width="12" customWidth="1" style="1"/></cols>
<sheetData>
<row r="1" ht="20" customHeight="1"><c r="A1" t="s"><v>0</v></c><c r="B1" s="1"><v>1.5</v></c><c r="C1" t="b"><v>1</v></c><c r="D1" t="e"><v>#DIV/0!</v></c></row>
<row r="2"><c r="A2"><f>SUM(B1:B1)+IF(C1,1,2)</f><v>2.5</v></c><c r="B2" t="str"><f>"a"&amp;A1</f><v>a</v></c><c r="C2" t="inlineStr"><is><t>inline</t></is></c></row>
<row r="3"><c r="A3"><f t="array" ref="A3:B4">{1,2;3,4}</f><v>1</v></c><c r="B3"><v>2</v></c><c r="A4"><v>3</v></c><c r="B4"><v>4</v></c></row>
<row r="4"><c r="C4"><f t="shared" ref="C4:C5" si="0">A2*2</f><v>5</v></c></row>
<row r="5"><c r="C5"><f t="shared" si="0"/><v>6</v></c></row>
</sheetData>
<mergeCells count="1"><mergeCell ref="E1:F2"/></mergeCells>
<conditionalFormatting sqref="B1:B5"><cfRule type="cellIs" dxfId="0" priority="1" operator="greaterThan"><formula>1</formula></cfRule></conditionalFormatting>
</worksheet>`

const fuzzStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="1"><numFmt numFmtId="164" formatCode="0.00&quot; €&quot;"/></numFmts>
<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="12"/><color rgb="FFFF0000"/><name val="Arial"/></font></fonts>
<fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FFFFFF00"/></patternFill></fill></fills>
<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color auto="1"/></left><right/><top/><bottom/><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="164" fontId="1" fillId="2" borderId="1" xfId="0" applyNumberFormat="1"><alignment horizontal="center" wrapText="1"/></xf></cellXfs>
<dxfs count="1"><dxf><font><b/></font></dxf></dxfs>
</styleSheet>`

const fuzzStrings = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="2"><si><t>shared</t></si><si><r><rPr><b/></rPr><t>rich</t></r><r><t xml:space="preserve"> text</t></r></si></sst>`

const fuzzWorkbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><workbookPr date1904="0"/><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets><definedNames><definedName name="Total">Sheet1!$A$1:$B$2</definedName></definedNames></workbook>`

// skeleton is a valid workbook whose workbook, sheet, styles and shared
// strings are the parts given.
func skeleton(t testing.TB, workbook, sheet, styles, shared []byte) []byte {
	t.Helper()
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(blank, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rels, err := p.Relationships("xl/workbook.xml")
	if err != nil {
		t.Fatal(err)
	}
	rels = append(rels,
		opc.Relationship{ID: "rIdStyles", Type: relStyles, Target: "styles.xml"},
		opc.Relationship{ID: "rIdStrings", Type: relSharedStrings, Target: "sharedStrings.xml"})
	for _, set := range []struct {
		name string
		data []byte
	}{{"xl/workbook.xml", workbook}, {"xl/worksheets/sheet1.xml", sheet}, {"xl/_rels/workbook.xml.rels", opc.MarshalRelationships(rels)}} {
		if err := p.Set(set.name, set.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Add("xl/styles.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml", styles); err != nil {
		t.Fatal(err)
	}
	if err := p.Add("xl/sharedStrings.xml", typeSharedStrings, shared); err != nil {
		t.Fatal(err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// exercise opens a workbook, calculates it and writes it: unchanged, the
// package is the original; rewritten sheet by sheet, it reads back as the
// same tree.
func exercise(t *testing.T, data []byte) {
	d, tree, err := Open(data)
	if err != nil {
		return
	}
	out, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	samePackage(t, "workbook", data, out)
	saved, err := d.save(tree, true)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatalf("rewritten workbook does not open: %v", err)
	}
	if err := sameTree(tree, again); err != nil {
		t.Fatal(err)
	}
	NewCalc(tree, formula.Options{})
	if _, err := d.Save(tree); err != nil {
		t.Fatal(err)
	}
}

func FuzzOpen(f *testing.F) {
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(blank)
	f.Add(skeleton(f, []byte(fuzzWorkbook), []byte(fuzzSheet), []byte(fuzzStyles), []byte(fuzzStrings)))
	f.Fuzz(exercise)
}

// FuzzParts fuzzes the parts of a valid workbook one at a time, so that
// the fuzzer spends no time on its package.
func FuzzParts(f *testing.F) {
	f.Add([]byte(fuzzWorkbook), []byte(fuzzSheet), []byte(fuzzStyles), []byte(fuzzStrings))
	f.Fuzz(func(t *testing.T, workbook, sheet, styles, shared []byte) {
		exercise(t, skeleton(t, workbook, sheet, styles, shared))
	})
}

// A formula nested too deeply for the calculator is kept as the file has it.
func TestDeepFormula(t *testing.T) {
	deep := strings.Repeat("(", 100000) + "1" + strings.Repeat(")", 100000)
	sheet := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1"><f>` + deep + `</f><v>1</v></c></row></sheetData></worksheet>`
	data := skeleton(t, []byte(fuzzWorkbook), []byte(sheet), []byte(fuzzStyles), []byte(fuzzStrings))
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if calc := NewCalc(tree, formula.Options{}); calc.Formulas() != 0 {
		t.Errorf("%d formulas calculated", calc.Formulas())
	}
	out, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	samePackage(t, "deep", data, out)
	saved, err := d.save(tree, true)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(saved, Limits)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := p.Read("xl/worksheets/sheet1.xml")
	if !strings.Contains(string(part), "<f>"+deep+"</f>") {
		t.Error("formula changed")
	}
}
