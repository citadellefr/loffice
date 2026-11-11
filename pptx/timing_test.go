package pptx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func par(delay string, children ...string) string {
	return fmt.Sprintf(`<p:par><p:cTn id="0" fill="hold"><p:stCondLst><p:cond delay="%s"/></p:stCondLst><p:childTnLst>%s</p:childTnLst></p:cTn></p:par>`,
		delay, strings.Join(children, ""))
}

// number gives the time nodes their ids, in the order of the document.
func number(xml string) string {
	id := 0
	return regexp.MustCompile(`<p:cTn id="0"`).ReplaceAllStringFunc(xml, func(string) string {
		id++
		return fmt.Sprintf(`<p:cTn id="%d"`, id)
	})
}

func effect(nodeType, target string) string {
	return fmt.Sprintf(`<p:par><p:cTn id="0" presetID="1" presetClass="entr" presetSubtype="0" fill="hold" grpId="0" nodeType="%s"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst><p:set><p:cBhvr><p:cTn id="0" dur="1" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst></p:cTn><p:tgtEl>%s</p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set></p:childTnLst></p:cTn></p:par>`,
		nodeType, target)
}

func spTgt(spid string) string {
	return `<p:spTgt spid="` + spid + `"/>`
}

func paragraph(spid string) string {
	return `<p:spTgt spid="` + spid + `"><p:txEl><p:pRg st="0" end="0"/></p:txEl></p:spTgt>`
}

// slideTiming is a p:timing of three animated shapes: the first at a click,
// the second after it, the third by paragraph at another click.
func slideTiming() string {
	clicks := par("indefinite",
		par("0", effect("clickEffect", spTgt("2"))),
		par("1", effect("afterEffect", spTgt("3"))),
	) + par("indefinite",
		par("0", effect("clickEffect", paragraph("4"))),
	)
	return number(fmt.Sprintf(`<p:timing><p:tnLst><p:par><p:cTn id="0" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst><p:seq concurrent="1" nextAc="seek"><p:cTn id="0" dur="indefinite" nodeType="mainSeq"><p:childTnLst>%s</p:childTnLst></p:cTn><p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst><p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst></p:seq></p:childTnLst></p:cTn></p:par></p:tnLst><p:bldLst><p:bldP spid="2" grpId="0" animBg="1"/><p:bldP spid="3" grpId="0" animBg="1"/><p:bldP spid="4" grpId="0" build="p"/></p:bldLst></p:timing>`,
		clicks))
}

func animatedShape(id int) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Forme %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="838200" y="%d"/><a:ext cx="2000000" cy="500000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Forme %d</a:t></a:r></a:p></p:txBody></p:sp>`,
		id, id, 1800000+600000*id, id)
}

// animatedSlide is a presentation whose slide has the shapes 2 to 5, the
// first three animated.
func animatedSlide(t *testing.T) (*Document, *ot.Tree) {
	t.Helper()
	_, _, data := openFile(t, "transitional.pptx")
	parts := rawParts(t, data)
	xml := string(parts["ppt/slides/slide1.xml"])
	extra := animatedShape(3) + animatedShape(4) + animatedShape(5)
	xml = strings.Replace(xml, "</p:spTree>", extra+"</p:spTree>", 1)
	xml = strings.Replace(xml, "</p:cSld>", "</p:cSld>"+slideTiming(), 1)
	parts["ppt/slides/slide1.xml"] = []byte(xml)
	d, tree, err := Open(zipOf(t, parts), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

// deleteShapes saves the slide without the shapes named, and gives back
// its XML.
func deleteShapes(t *testing.T, names ...string) string {
	t.Helper()
	d, tree := animatedSlide(t)
	slide := slidesOf(tree)[0].ID
	var e ot.Edit
	for _, name := range names {
		for _, c := range tree.Children(slide) {
			if string(c.Attrs["name"]) == `"`+name+`"` {
				e = append(e, ot.Change{Op: ot.OpDel, ID: c.ID})
			}
		}
	}
	if len(e) != len(names) {
		t.Fatalf("%d shapes found for %v", len(e), names)
	}
	apply(t, d, tree, e)
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("LOFFICE_EDITED"); out != "" {
		path := filepath.Join(out, "timing", strings.Join(names, "-")+".pptx")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, saved, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Open(saved, opc.Limits{}); err != nil {
		t.Fatal(err)
	}
	return string(rawParts(t, saved)["ppt/slides/slide1.xml"])
}

func timingPart(xml string) string {
	from, to := strings.Index(xml, "<p:timing"), strings.Index(xml, "</p:timing>")
	if from < 0 || to < 0 {
		return ""
	}
	return xml[from : to+len("</p:timing>")]
}

func spidsOf(timing string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<p:spTgt spid="(\d+)"`).FindAllStringSubmatch(timing, -1) {
		out = append(out, m[1])
	}
	return out
}

func buildsOf(timing string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<p:bldP spid="(\d+)"`).FindAllStringSubmatch(timing, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestDeleteAnimatedShape(t *testing.T) {
	timing := timingPart(deleteShapes(t, "Forme 3"))
	if got := fmt.Sprint(spidsOf(timing)); got != "[2 4]" {
		t.Fatalf("targets %s", got)
	}
	if got := fmt.Sprint(buildsOf(timing)); got != "[2 4]" {
		t.Fatalf("builds %s", got)
	}
	for _, want := range []string{
		`nodeType="clickEffect"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst><p:set><p:cBhvr><p:cTn id="6" dur="1" fill="hold"><p:stCondLst><p:cond delay="0"/></p:stCondLst></p:cTn><p:tgtEl><p:spTgt spid="2"/>`,
		`<p:spTgt spid="4"><p:txEl><p:pRg st="0" end="0"/></p:txEl></p:spTgt>`,
		`<p:cond delay="indefinite"/>`,
		`<p:bldP spid="4" grpId="0" build="p"/>`,
	} {
		if !strings.Contains(timing, want) {
			t.Errorf("%s missing from %s", want, timing)
		}
	}
	if strings.Contains(timing, "afterEffect") || strings.Count(timing, `delay="indefinite"/></p:stCondLst>`) != 2 {
		t.Errorf("the group of the deleted shape stays: %s", timing)
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`<p:cTn id="(\d+)"`).FindAllStringSubmatch(timing, -1) {
		if ids[m[1]] {
			t.Errorf("id %s twice", m[1])
		}
		ids[m[1]] = true
	}
}

func TestDeleteClickShapeKeepsTheChain(t *testing.T) {
	timing := timingPart(deleteShapes(t, "Titre 1"))
	if got := fmt.Sprint(spidsOf(timing)); got != "[3 4]" {
		t.Fatalf("targets %s", got)
	}
	if got := fmt.Sprint(buildsOf(timing)); got != "[3 4]" {
		t.Fatalf("builds %s", got)
	}
	if strings.Contains(timing, "afterEffect") || strings.Count(timing, "clickEffect") != 2 {
		t.Errorf("the effect that follows does not start at the click: %s", timing)
	}
}

func TestDeleteParagraphShape(t *testing.T) {
	timing := timingPart(deleteShapes(t, "Forme 4"))
	if got := fmt.Sprint(spidsOf(timing)); got != "[2 3]" {
		t.Fatalf("targets %s", got)
	}
	if got := fmt.Sprint(buildsOf(timing)); got != "[2 3]" {
		t.Fatalf("builds %s", got)
	}
	if strings.Count(timing, `delay="indefinite"/></p:stCondLst>`) != 1 {
		t.Errorf("the click of the deleted shape stays: %s", timing)
	}
}

func TestDeleteEveryAnimatedShape(t *testing.T) {
	xml := deleteShapes(t, "Titre 1", "Forme 3", "Forme 4")
	if strings.Contains(xml, "<p:timing") || !strings.Contains(xml, "Forme 5") {
		t.Fatalf("timing left: %s", timingPart(xml))
	}
}

func TestDeleteStillShapeKeepsTheTimingAsIs(t *testing.T) {
	if got := timingPart(deleteShapes(t, "Forme 5")); got != slideTiming() {
		t.Fatalf("timing changed:\n%s", got)
	}
}

func prune(t *testing.T, timing string, live ...string) string {
	t.Helper()
	var shapes string
	for _, id := range live {
		shapes += `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name=""/></p:nvSpPr></p:sp>`
	}
	doc, err := xmldom.Parse([]byte(`<p:sld xmlns:p="` + pNS + `" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld>` + timing + `</p:sld>`))
	if err != nil {
		t.Fatal(err)
	}
	spTree := doc.Root.Child(pNS, "cSld").Child(pNS, "spTree")
	dropStaleTiming(doc.Root, spTree)
	return string(doc.Root.Bytes())
}

// With the shape that started a group gone, the effect that went along with
// it starts the group.
func TestDeleteGroupStarter(t *testing.T) {
	timing := number(`<p:timing><p:tnLst><p:par><p:cTn id="0" nodeType="tmRoot"><p:childTnLst><p:seq><p:cTn id="0" nodeType="mainSeq"><p:childTnLst>` +
		par("indefinite", par("0", effect("clickEffect", spTgt("2")), effect("withEffect", spTgt("3")), effect("withEffect", spTgt("4")))) +
		`</p:childTnLst></p:cTn></p:seq></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`)
	got := prune(t, timing, "3", "4")
	if strings.Count(got, "clickEffect") != 1 || strings.Count(got, "withEffect") != 1 || strings.Contains(got, `spid="2"`) {
		t.Fatalf("%s", got)
	}
	if !regexp.MustCompile(`nodeType="clickEffect">.*<p:spTgt spid="3"/>.*nodeType="withEffect">.*<p:spTgt spid="4"/>`).MatchString(got) {
		t.Fatalf("%s", got)
	}
}
