package legacy

import (
	"slices"
	"strings"
	"testing"
)

// has fails unless text holds each of the parts, JSON escapes of <, > and &
// apart.
func has(t *testing.T, what, text string, parts ...string) {
	t.Helper()
	text = strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&").Replace(text)
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Errorf("%s lacks %s: %s", what, p, text)
		}
	}
}

// kept fails when the conversion still tells it lost what it now converts.
func kept(t *testing.T, lost []Loss, converted ...Loss) {
	t.Helper()
	for _, l := range converted {
		if slices.Contains(lost, l) {
			t.Errorf("%q is told lost though it is converted: %v", l, lost)
		}
	}
}

func TestWorkbookRichText(t *testing.T) {
	tree, lost := converted(t, "12843-2.xls")
	kept(t, lost, RichText)
	has(t, "B43", fields(t, sheetNamed(t, tree, "questions"), "B43"), `<rPr>`, `superscript`, `Times New Roman`)
}

func TestWorkbookLinks(t *testing.T) {
	tree, lost := converted(t, "44958.xls")
	kept(t, lost, Links)
	has(t, "A1", fields(t, sheetNamed(t, tree, "PROMO"), "A1"), `"link":{"url":"http://www.ozgrid.com/Services/ExcelAdd-insPage.htm"}`)
	tree, _ = converted(t, "46445.xls")
	has(t, "A3", fields(t, sheetNamed(t, tree, "Sheet1"), "A3"), `"link":{"loc":"Sheet2!A1"}`)
}

func TestWorkbookNotes(t *testing.T) {
	tree, lost := converted(t, "29982.xls")
	kept(t, lost, Comments)
	has(t, "A63", fields(t, sheetNamed(t, tree, "Plan1"), "A63"), `"author":"Gilsinei Hansen"`, `Implementar os conceitos de TOC`)
}

func TestWorkbookPrint(t *testing.T) {
	tree, lost := converted(t, "15228.xls")
	kept(t, lost, Print)
	has(t, "BlockExposure", string(sheetNamed(t, tree, "BlockExposure").Attrs["print"]), `"land":true`, `"fit":[1,1]`, `"across":true`, `"foot":"&L`)
	has(t, "RepLag", string(sheetNamed(t, tree, "RepLag").Attrs["print"]), `"cols":[24]`)
	tree, _ = converted(t, "29982.xls")
	has(t, "Plan1", string(sheetNamed(t, tree, "Plan1").Attrs["print"]), `"paper":9`, `"fit":[1,6]`, `"l":0.3937007874015748`, `"ch":true`)
}

func TestWorkbookProtection(t *testing.T) {
	tree, lost := converted(t, "15556.xls")
	kept(t, lost, Protection)
	has(t, "protect", string(sheetNamed(t, tree, "2270-170-1").Attrs["protect"]), `"pass":"CA7F"`, `"objects":true`, `"scenarios":true`, `selectLockedCells`)
}

func TestWorkbookFilters(t *testing.T) {
	tree, lost := converted(t, "46250.xls")
	kept(t, lost, Filters)
	has(t, "filter", string(sheetNamed(t, tree, "Item Info").Attrs["filter"]), `"ref":"A1:I174"`, `"cols":[{"col":5,"vals":["Enchilada"]}]`)
}

// The rules of Excel 97 on the value of a cell become formulas, and those
// only Excel 2007 holds are told lost.
func TestWorkbookConditional(t *testing.T) {
	tree, lost := converted(t, "WithConditionalFormatting.xls")
	kept(t, lost, Conditional)
	has(t, "cf", string(sheetNamed(t, tree, "CF").Attrs["cf"]),
		`"ref":"A1:A8","type":"formula","pri":1,"f":["A1>(3)"]`, `"b":true,"color":{"rgb":"008000"}`,
		`AND(B1>=(\"A\"),B1<=(\"AAA\"))`, `"u":"single"`, `"fill":{"pattern":"solid","fg":{"rgb":"CCCCFF"}}`)
	tree, lost = converted(t, "ConditionalFormattingSamples.xls")
	if !slices.Contains(lost, Conditional) {
		t.Errorf("the scales and icons of Excel 2007 are not told lost: %v", lost)
	}
	has(t, "Banded rows", string(sheetNamed(t, tree, "Banded rows").Attrs["cf"]), `"f":["MOD(ROW(),2)=1"]`, `"border":{"t":{"style":"thin"`)
}

func TestWorkbookValidation(t *testing.T) {
	tree, lost := converted(t, "ConditionalFormattingSamples.xls")
	has(t, "Products2", string(sheetNamed(t, tree, "Products2").Attrs["valid"]),
		`"ref":"G5","type":"list","f1":"\"$100,$200,$300,$400,$500\""`, `"ref":"G2","type":"list","f1":"\"Dairy,Produce,Grain\""`)
	kept(t, lost, Validation)
	tree, _ = converted(t, "39634.xls")
	has(t, "valid", string(sheetNamed(t, tree, "14. EQUIPMENT Data").Attrs["valid"]),
		`"type":"decimal","op":"greaterThan","f1":"0"`, `"ref":"C6:C8","type":"list","f1":"$C$54:$C$59"`, `"errTitle":"List"`)
}
