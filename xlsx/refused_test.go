package xlsx

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

const betaMain = `"http://schemas.microsoft.com/office/excel/2006/2"`

// withDoctype is transitional.xlsx with a document type declaration in each
// part of the given names.
func withDoctype(t *testing.T, doctype string, names ...string) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/xlsx/transitional.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	parts := rawParts(t, data)
	for _, name := range names {
		parts[name] = bytes.Replace(parts[name], []byte("?>"), []byte("?>"+doctype), 1)
		if !bytes.Contains(parts[name], []byte(doctype)) {
			t.Fatalf("%s has no prolog", name)
		}
	}
	return zipOf(t, parts)
}

func TestDoctypeWithoutEntities(t *testing.T) {
	data, err := os.ReadFile("../testdata/xlsx/transitional.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, want, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, doctype := range []string{"<!DOCTYPE workbook>", "<!DOCTYPE workbook [<!ELEMENT workbook ANY>]>"} {
		data := withDoctype(t, doctype, "xl/workbook.xml", "xl/worksheets/sheet1.xml")
		d, tree, err := Open(data, Limits{})
		if err != nil {
			t.Fatal(doctype, err)
		}
		if err := sameTree(want, tree); err != nil {
			t.Fatal(doctype, err)
		}
		if saved, err := d.Save(tree); err != nil || !bytes.Equal(saved, data) {
			t.Fatal(doctype, "unedited workbook saved differently", err)
		}
	}
}

func TestEntitiesRefused(t *testing.T) {
	for doctype, want := range map[string]string{
		`<!DOCTYPE workbook [<!ENTITY a "b">]>`:                    "xlsx: xl/workbook.xml: xmltok: syntax error at offset 92: entity declarations are not allowed",
		`<!DOCTYPE workbook [<!ENTITY a SYSTEM "file:///etc/a">]>`: "xlsx: xl/workbook.xml: xmltok: syntax error at offset 111: entity declarations are not allowed",
		`<!DOCTYPE workbook PUBLIC "-//A//EN" "a.dtd">`:            "xlsx: xl/workbook.xml: xmltok: syntax error at offset 100: external document type definitions are not allowed",
	} {
		_, _, err := Open(withDoctype(t, doctype, "xl/workbook.xml"), Limits{})
		if err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", doctype, err, want)
		}
	}
}

func TestBetaWorkbook(t *testing.T) {
	data, err := os.ReadFile("../testdata/xlsx/transitional.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	parts := rawParts(t, data)
	parts["xl/workbook.xml"] = []byte(strings.ReplaceAll(string(parts["xl/workbook.xml"]), transitionalMain, betaMain))
	if _, _, err := Open(zipOf(t, parts), Limits{}); !errors.Is(err, ErrNotWorkbook) || err.Error() != "xlsx: not a workbook" {
		t.Fatal(err)
	}
}
