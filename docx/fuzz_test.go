package docx

import (
	"os"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

// fuzzed are the parts of the skeleton FuzzOpen puts its input in.
var fuzzed = []string{"word/document.xml", "word/styles.xml", "word/numbering.xml", "word/footnotes.xml", "word/header1.xml", "word/comments.xml", "word/commentsExtended.xml"}

const numbering = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
	`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`

const fuzzBody = `<w:p><w:pPr><w:pStyle w:val="Heading1"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Titre</w:t></w:r></w:p>` +
	commented + revised +
	`<w:p><w:hyperlink r:id="rId3"><w:r><w:t>lien</w:t></w:r></w:hyperlink><w:r><w:footnoteReference w:id="1"/></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
	`<w:tbl><w:tblPr><w:tblStyle w:val="Grid"/></w:tblPr><w:tblGrid><w:gridCol w:w="4000"/><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>a</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p/></w:tc><w:tc><w:p/></w:tc></w:tr></w:tbl>` +
	`<w:p><w:r><w:drawing><wp:inline xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><wp:extent cx="127000" cy="127000"/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:blipFill><a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="rId4"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`

// skeleton is a valid document of that body, with every part FuzzOpen puts
// its input in.
func skeleton(t testing.TB, body string) *opc.Package {
	t.Helper()
	pkg, err := opc.Open(withComments(t, body), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rels, _ := pkg.Read("word/_rels/document.xml.rels")
	rels = append(rels[:len(rels)-len("</Relationships>")], `<Relationship Id="rId10" Type="`+relNumbering+`" Target="numbering.xml"/></Relationships>`...)
	if err := pkg.Set("word/_rels/document.xml.rels", rels); err != nil {
		t.Fatal(err)
	}
	if err := pkg.Add("word/numbering.xml", typeNumbering, []byte(numbering)); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func blankDocx(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("../opc/testdata/blank.docx")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func FuzzOpen(f *testing.F) {
	for i, name := range fuzzed {
		pkg := skeleton(f, fuzzBody)
		data, err := pkg.Read(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint8(i), data)
	}
	blank, err := opc.Open(blankDocx(f), opc.Limits{})
	if err != nil {
		f.Fatal(err)
	}
	main, _ := blank.Read("word/document.xml")
	f.Add(uint8(0), main)
	base, err := skeleton(f, fuzzBody).Bytes()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		pkg, err := opc.Open(base, opc.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if err := pkg.Set(fuzzed[int(which)%len(fuzzed)], data); err != nil {
			t.Fatal(err)
		}
		in, err := pkg.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		d, tree, err := Open(in, opc.Limits{})
		if err != nil {
			return
		}
		saved, err := save(d, tree, true)
		if err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		if _, _, err := Open(saved, opc.Limits{}); err != nil {
			t.Fatalf("rewritten package does not open: %v", err)
		}
		if e := edits(tree); len(e) > 0 && d.Check(tree, e, "Bref") == nil && tree.Apply(e) == nil {
			saved, err := d.Save(tree)
			if err != nil {
				t.Fatalf("save: %v", err)
			}
			if _, _, err := Open(saved, opc.Limits{}); err != nil {
				t.Fatalf("edited package does not open: %v", err)
			}
		}
	})
}
