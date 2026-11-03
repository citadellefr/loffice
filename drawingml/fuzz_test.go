package drawingml

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

const shape = `<p:spPr xmlns:a="` + NS + `" xmlns:p="urn:p" xmlns:r="` + RelNS + `"><a:xfrm rot="900000" flipH="1"><a:off x="1" y="2"/><a:ext cx="3" cy="4"/></a:xfrm>` +
	`<a:custGeom><a:avLst/><a:rect l="l" t="t" r="r" b="b"/><a:pathLst><a:path w="100" h="100"><a:moveTo><a:pt x="0" y="0"/></a:moveTo><a:lnTo><a:pt x="100" y="100"/></a:lnTo><a:arcTo wR="50" hR="50" stAng="0" swAng="5400000"/><a:close/></a:path></a:pathLst></a:custGeom>` +
	`<a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:schemeClr val="accent1"><a:alpha val="50000"/></a:schemeClr></a:gs><a:gs pos="100000"><a:srgbClr val="FF0000"/></a:gs></a:gsLst><a:path path="circle"><a:fillToRect l="50000" t="50000" r="50000" b="50000"/></a:path></a:gradFill>` +
	`<a:ln w="12700" cap="rnd"><a:pattFill prst="dkDnDiag"><a:fgClr><a:srgbClr val="000000"/></a:fgClr><a:bgClr><a:sysClr val="window"/></a:bgClr></a:pattFill><a:prstDash val="sysDash"/><a:round/><a:tailEnd type="triangle"/></a:ln></p:spPr>`

const picture = `<p:blipFill xmlns:a="` + NS + `" xmlns:p="urn:p" xmlns:r="` + RelNS + `"><a:blip r:embed="rId2"><a:duotone><a:srgbClr val="000000"/><a:srgbClr val="FFFFFF"/></a:duotone></a:blip><a:srcRect l="10" t="20"/><a:tile tx="1" ty="2" sx="3" sy="4" flip="x" algn="ctr"/></p:blipFill>`

const preset = `<p:spPr xmlns:a="` + NS + `" xmlns:p="urn:p"><a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val 20000"/></a:avLst></a:prstGeom><a:solidFill><a:prstClr val="black"><a:lumMod val="50000"/><a:lumOff val="10000"/></a:prstClr></a:solidFill></p:spPr>`

const styles = `<a:lstStyle xmlns:a="` + NS + `"><a:lvl1pPr marL="0" indent="-342900" algn="l"><a:lnSpc><a:spcPct val="90000"/></a:lnSpc><a:spcBef><a:spcPts val="1000"/></a:spcBef><a:buClr><a:srgbClr val="FF0000"/></a:buClr><a:buSzPct val="100000"/><a:buFont typeface="Arial"/><a:buChar char="•"/><a:tabLst><a:tab pos="100" algn="l"/></a:tabLst>` +
	`<a:defRPr sz="1800" b="1" i="1" u="sng" strike="sngStrike" baseline="30000" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:hlinkClick r:id="rId1" action="ppaction://noaction" xmlns:r="` + RelNS + `"/></a:defRPr></a:lvl1pPr></a:lstStyle>`

func FuzzReadWrite(f *testing.F) {
	for _, seed := range []string{body, shape, picture, preset, styles, `<a:bodyPr xmlns:a="` + NS + `" wrap="square" lIns="0" anchor="ctr" rtlCol="0"><a:normAutofit fontScale="90000"/></a:bodyPr>`} {
		f.Add([]byte(seed))
	}
	pkg, err := os.ReadFile("../opc/testdata/blank.pptx")
	if err != nil {
		f.Fatal(err)
	}
	p, err := opc.Open(pkg, opc.Limits{})
	if err != nil {
		f.Fatal(err)
	}
	for _, name := range []string{"ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml", "ppt/slides/slide1.xml"} {
		data, err := p.Read(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := xmldom.Parse(data)
		if err != nil {
			return
		}
		spaces := map[string]string{"a": NS, "r": RelNS}
		maps.Copy(spaces, doc.Root.Spaces())
		walk(doc.Root, func(e *xmldom.Element) {
			link := func(rid string) string { return rid }
			switch e.Local {
			case "txBody", "txPr", "rich":
				flow := Flow(e, link)
				SetFlow(e, flow, fragments(spaces), nil)
				if again := Flow(e, link); !equalFlows(flow, again) {
					t.Fatalf("a text body reads %v once rewritten, was %v", again, flow)
				}
			case "spPr":
				old := ShapeProps(e, link)
				for _, c := range shapeChanges {
					values := maps.Clone(old)
					values[c.key] = json.RawMessage(c.value)
					SetShapeProps(e, old, values, func(string) string { return "rId1" })
					old = ShapeProps(e, link)
				}
			case "rPr", "defRPr", "endParaRPr":
				props := RunProps(e, link)
				SetRunProps(e, props, props)
			case "pPr":
				props := ParaProps(e)
				SetParaProps(e, props, props)
			case "bodyPr":
				props := BodyProps(e)
				SetBodyProps(e, props, props)
			case "lstStyle", "titleStyle", "bodyStyle", "otherStyle":
				ListStyle(e)
			case "blipFill", "bgPr":
				FillIn(e, link)
			case "blip":
				ReadBlip(e, link)
			case "xfrm":
				ReadXfrm(e)
			case "ln":
				ReadLine(e)
			default:
				ColorIn(e)
			}
		})
	})
}

func equalFlows(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
