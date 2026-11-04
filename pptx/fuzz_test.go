package pptx

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// fuzzed are the parts of the skeleton FuzzOpen puts its input in.
var fuzzed = []string{
	"ppt/slides/slide1.xml", "ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml", "ppt/presentation.xml",
	"ppt/theme/theme1.xml", "ppt/commentAuthors.xml", "ppt/comments/comment1.xml", "ppt/tableStyles.xml",
}

const (
	fuzzNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	group  = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`

	fuzzSlide = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<p:sld ` + fuzzNS + `><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="F0F0F0"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>` + group +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Titre"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="fr-FR" b="1" sz="3200"><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:rPr><a:t>Titre</a:t></a:r><a:br/><a:r><a:rPr><a:hlinkClick r:id="rId9"/></a:rPr><a:t>lien</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Forme"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm rot="900000"><a:off x="914400" y="914400"/><a:ext cx="1828800" cy="914400"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val 16667"/></a:avLst></a:prstGeom>` +
		`<a:gradFill><a:gsLst><a:gs pos="0"><a:srgbClr val="FF0000"/></a:gs><a:gs pos="100000"><a:schemeClr val="accent2"><a:lumMod val="50000"/></a:schemeClr></a:gs></a:gsLst><a:lin ang="5400000"/></a:gradFill><a:ln w="12700"><a:solidFill><a:srgbClr val="000000"/></a:solidFill><a:prstDash val="dash"/></a:ln></p:spPr>` +
		`<p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle><a:lvl1pPr><a:defRPr sz="1800"/></a:lvl1pPr></a:lstStyle><a:p><a:pPr lvl="1"><a:buChar char="•"/></a:pPr><a:r><a:t>Texte</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:pic><p:nvPicPr><p:cNvPr id="4" name="Image"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="rId8"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></a:xfrm><a:prstGeom prst="rect"/></p:spPr></p:pic>` +
		`<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="5" name="Tableau"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1"><a:tableStyleId>{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}</a:tableStyleId></a:tblPr><a:tblGrid><a:gridCol w="1000"/><a:gridCol w="1000"/></a:tblGrid>` +
		`<a:tr h="300"><a:tc gridSpan="2"><a:txBody><a:bodyPr/><a:p><a:r><a:t>a</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc><a:tc hMerge="1"><a:txBody><a:bodyPr/><a:p/></a:txBody><a:tcPr/></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>` +
		`<p:grpSp><p:nvGrpSpPr><p:cNvPr id="6" name="Groupe"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:cxnSp><p:nvCxnSpPr><p:cNvPr id="7" name="Lien"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr><p:spPr><a:prstGeom prst="line"/></p:spPr></p:cxnSp></p:grpSp>` +
		`</p:spTree></p:cSld><p:transition spd="med"><p:fade/></p:transition></p:sld>`

	fuzzMaster = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<p:sldMaster ` + fuzzNS + `><p:cSld><p:spTree>` + group +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Titre"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="504000" y="226080"/><a:ext cx="9071640" cy="946440"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
		`<p:txBody><a:bodyPr anchor="ctr"><a:noAutofit/></a:bodyPr><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="en-US" sz="4400"/><a:t>Titre</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Corps"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="504000" y="1326600"/><a:ext cx="9071640" cy="3288240"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:pPr marL="432000" indent="-324000"><a:buChar char="•"/></a:pPr><a:r><a:rPr lang="en-US" sz="3200"/><a:t>Texte</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId2"/></p:sldLayoutIdLst></p:sldMaster>`

	fuzzLayout = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<p:sldLayout ` + fuzzNS + ` type="title" preserve="1"><p:cSld name="Default"><p:spTree>` + group +
		`<p:sp><p:nvSpPr><p:cNvPr id="7" name="Titre"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:p><a:endParaRPr lang="en-US"/></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sldLayout>`

	fuzzAuthors  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<p:cmAuthorLst ` + fuzzNS + `><p:cmAuthor id="0" name="Alice" initials="A" lastIdx="2" clrIdx="0"/></p:cmAuthorLst>`
	fuzzComments = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<p:cmLst ` + fuzzNS + `><p:cm authorId="0" dt="2026-09-01T10:00:00.000" idx="1"><p:pos x="10" y="20"/><p:text>Pourquoi ?</p:text></p:cm></p:cmLst>`
	fuzzStyles   = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<a:tblStyleLst ` + fuzzNS + ` def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"><a:tblStyle styleId="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}" styleName="Medium Style 2 - Accent 1"><a:wholeTbl><a:tcTxStyle><a:fontRef idx="minor"/></a:tcTxStyle><a:tcStyle><a:tcBdr/></a:tcStyle></a:wholeTbl></a:tblStyle></a:tblStyleLst>`
)

func blankPptx(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("../opc/testdata/blank.pptx")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fuzzSkeleton is the blank presentation, with a master and a layout cut to
// what the slide inherits, and a slide that has shapes, a table,
// comments and every part FuzzOpen puts its input in.
func fuzzSkeleton(t testing.TB) *opc.Package {
	t.Helper()
	pkg, err := opc.Open(blankPptx(t), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rels, _ := pkg.Read("ppt/_rels/presentation.xml.rels")
	rels = []byte(strings.Replace(string(rels), "</Relationships>", `<Relationship Id="rId5" Type="`+relCommentAuthors+`" Target="commentAuthors.xml"/><Relationship Id="rId6" Type="`+relTableStyles+`" Target="tableStyles.xml"/></Relationships>`, 1))
	slideRels, _ := pkg.Read("ppt/slides/_rels/slide1.xml.rels")
	slideRels = []byte(strings.Replace(string(slideRels), "</Relationships>", `<Relationship Id="rId7" Type="`+relComments+`" Target="../comments/comment1.xml"/>`+
		`<Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image1.png"/>`+
		`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/" TargetMode="External"/></Relationships>`, 1))
	for _, err := range []error{
		pkg.Set("ppt/_rels/presentation.xml.rels", rels),
		pkg.Set("ppt/slides/_rels/slide1.xml.rels", slideRels),
		pkg.Set("ppt/slideMasters/slideMaster1.xml", []byte(fuzzMaster)),
		pkg.Set("ppt/slideLayouts/slideLayout1.xml", []byte(fuzzLayout)),
		pkg.Set("ppt/slides/slide1.xml", []byte(fuzzSlide)),
		pkg.Add("ppt/commentAuthors.xml", typeAuthors, []byte(fuzzAuthors)),
		pkg.Add("ppt/comments/comment1.xml", typeComments, []byte(fuzzComments)),
		pkg.Add("ppt/tableStyles.xml", "application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml", []byte(fuzzStyles)),
		pkg.Add("ppt/media/image1.png", "image/png", []byte("\x89PNG\r\n\x1a\n")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return pkg
}

func FuzzOpen(f *testing.F) {
	pkg := fuzzSkeleton(f)
	for i, name := range fuzzed {
		data, err := pkg.Read(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint8(i), data)
	}
	base := stored(f, fuzzSkeleton(f))
	f.Fuzz(func(t *testing.T, which uint8, data []byte) { fuzzOpen(t, base, which, data) })
}

// stored is the package with its parts uncompressed, which parts replaced
// later stay: FuzzOpen compresses nothing.
func stored(t testing.TB, pkg *opc.Package) []byte {
	t.Helper()
	data, err := pkg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(w, r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func fuzzOpen(t testing.TB, base []byte, which uint8, data []byte) {
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
	slides := slidesOf(tree)
	if len(slides) == 0 {
		return
	}
	e := ot.Edit{commentAdded("fuzz", slides[0].ID, "Bob", "", "Commentaire")}
	if n := firstText(tree, slides[0].ID); n != nil {
		e = append(e, ot.Change{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{{Insert: "Bref ", Attrs: ot.Attrs{"b": "1"}}}})
	}
	if d.Check(tree, e, "Bob") != nil || tree.Apply(e) != nil {
		return
	}
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, _, err := Open(saved, opc.Limits{}); err != nil {
		t.Fatalf("edited package does not open: %v", err)
	}
}

func BenchmarkFuzzOpen(b *testing.B) {
	pkg := fuzzSkeleton(b)
	base := stored(b, pkg)
	seeds := make([][]byte, len(fuzzed))
	for i, name := range fuzzed {
		var err error
		if seeds[i], err = pkg.Read(name); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; b.Loop(); i++ {
		fuzzOpen(b, base, uint8(i%len(fuzzed)), seeds[i%len(fuzzed)])
	}
}
