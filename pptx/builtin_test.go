package pptx

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

// families are Office's built-in table styles: the id of each family's
// style without accent, then those of accents 1 to 6. Dark Style 2 pairs
// accents: 1 with 2, 3 with 4, 5 with 6.
var families = []struct {
	name string
	ids  []string
}{
	{"Themed Style 1", []string{"{2D5ABB26-0587-4C30-8999-92F81FD0307C}", "{3C2FFA5D-87B4-456A-9821-1D502468CF0F}", "{284E427A-3D55-4303-BF80-6455036E1DE7}", "{69C7853C-536D-4A76-A0AE-DD22124D55A5}", "{775DCB02-9BB8-47FD-8907-85C794F793BA}", "{35758FB7-9AC5-4552-8A53-C91805E547FA}", "{08FB837D-C827-4EFA-A057-4D05807E0F7C}"}},
	{"Themed Style 2", []string{"{5940675A-B579-460E-94D1-54222C63F5DA}", "{D113A9D2-9D6B-4929-AA2D-F23B5EE8CBE7}", "{18603FDC-E32A-4AB5-989C-0864C3EAD2B8}", "{306799F8-075E-4A3A-A7F6-7FBC6576F1A4}", "{E269D01E-BC32-4049-B463-5C60D7B0CCD2}", "{327F97BB-C833-4FB7-BDE5-3F7075034690}", "{638B1855-1B75-4FBE-930C-398BA8C253C6}"}},
	{"Light Style 1", []string{"{9D7B26C5-4107-4FEC-AEDC-1716B250A1EF}", "{3B4B98B0-60AC-42C2-AFA5-B58CD77FA1E5}", "{0E3FDE45-AF77-4B5C-9715-49D594BDF05E}", "{C083E6E3-FA7D-4D7B-A595-EF9225AFEA82}", "{D27102A9-8310-4765-A935-A1911B00CA55}", "{5FD0F851-EC5A-4D38-B0AD-8093EC10F338}", "{68D230F3-CF80-4859-8CE7-A43EE81993B5}"}},
	{"Light Style 2", []string{"{7E9639D4-E3E2-4D34-9284-5A2195B3D0D7}", "{69012ECD-51FC-41F1-AA8D-1B2483CD663E}", "{72833802-FEF1-4C79-8D5D-14CF1EAF98D9}", "{F2DE63D5-997A-4646-A377-4702673A728D}", "{17292A2E-F333-43FB-9621-5CBBE7FDCDCB}", "{5A111915-BE36-4E01-A7E5-04B1672EAD32}", "{912C8C85-51F0-491E-9774-3900AFEF0FD7}"}},
	{"Light Style 3", []string{"{616DA210-FB5B-4158-B5E0-FEB733F419BA}", "{BC89EF96-8CEA-46FF-86C4-4CE0E7609802}", "{5DA37D80-6434-44D0-A028-1B22A696006F}", "{8799B23B-EC83-4686-B30A-512413B5E67A}", "{ED083AE6-46FA-4A59-8FB0-9F97EB10719F}", "{BDBED569-4797-4DF1-A0F4-6AAB3CD982D8}", "{E8B1032C-EA38-4F05-BA0D-38AFFFC7BED3}"}},
	{"Medium Style 1", []string{"{793D81CF-94F2-401A-BA57-92F5A7B2D0C5}", "{B301B821-A1FF-4177-AEE7-76D212191A09}", "{9DCAF9ED-07DC-4A11-8D7F-57B35C25682E}", "{1FECB4D8-DB02-4DC6-A0A2-4F2EBAE1DC90}", "{1E171933-4619-4E11-9A3F-F7608DF75F80}", "{FABFCF23-3B69-468F-B69F-88F6DE6A72F2}", "{10A1B5D5-9B99-4C35-A422-299274C87663}"}},
	{"Medium Style 2", []string{"{073A0DAA-6AF3-43AB-8588-CEC1D06C72B9}", "{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}", "{21E4AEA4-8DFA-4A89-87EB-49C32662AFE0}", "{F5AB1C69-6EDB-4FF4-983F-18BD219EF322}", "{00A15C55-8517-42AA-B614-E9B94910E393}", "{7DF18680-E054-41AD-8BC1-D1AEF772440D}", "{93296810-A885-4BE3-A3E7-6D5BEEA58F35}"}},
	{"Medium Style 3", []string{"{8EC20E35-A176-4012-BC5E-935CFFF8708E}", "{6E25E649-3F16-4E02-A733-19D2CDBF48F0}", "{85BE263C-DBD7-4A20-BB59-AAB30ACAA65A}", "{EB344D84-9AFB-497E-A393-DC336BA19D2E}", "{EB9631B5-78F2-41C9-869B-9F39066F8104}", "{74C1A8A3-306A-4EB7-A6B1-4F7E0EB9C5D6}", "{2A488322-F2BA-4B5B-9748-0D474271808F}"}},
	{"Medium Style 4", []string{"{D7AC3CCA-C797-4891-BE02-D94E43425B78}", "{69CF1AB2-1976-4502-BF36-3FF5EA218861}", "{8A107856-5554-42FB-B03E-39F5DBC370BA}", "{0505E3EF-67EA-436B-97B2-0124C06EBD24}", "{C4B1156A-380E-4F78-BDF5-A606A8083BF9}", "{22838BEF-8BB2-4498-84A7-C5851F593DF1}", "{16D9F66E-5EB9-4882-86FB-DCBF35E3C3E4}"}},
	{"Dark Style 1", []string{"{E8034E78-7F5D-4C2E-B375-FC64B27BC917}", "{125E5076-3810-47DD-B79F-674D7AD40C01}", "{37CE84F3-28C3-443E-9E96-99CF82512B78}", "{D03447BB-5D67-496B-8E87-E561075AD55C}", "{E929F9F4-4A8F-4326-A1B4-22849713DDAB}", "{8FD4443E-F989-4FC4-A0C8-D5A2AF1F390B}", "{AF606853-7671-496A-8E4F-DF71F8EC918B}"}},
	{"Dark Style 2", []string{"{5202B0CA-FC54-4496-8BCA-5EF66A818D29}", "{0660B408-B3CF-4A94-85FC-2B1E0A45F4A2}", "", "{91EBBBCC-DAD2-459C-BE2E-F6DE35CF9A28}", "", "{46F890A9-2807-4EBB-B81D-B2AA78EC7F39}", ""}},
}

// TestBuiltinStyles writes tablestyles.json, Office's built-in table
// styles, from those PowerPoint wrote into the presentations of the
// corpus: each family is one style whose accent changes, which gives the
// styles no presentation holds. A family's style without accent that no
// presentation holds is drawn in dk1, as LibreOffice draws it; styles
// written with RGB colors were rewritten by another program and are left
// out.
func TestBuiltinStyles(t *testing.T) {
	seen := map[string]map[string]int{}
	files, _ := filepath.Glob("../corpus/files/*/*.pptx")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		pkg, err := opc.Open(data, Limits)
		if err != nil {
			continue
		}
		part, err := pkg.Read("ppt/tableStyles.xml")
		if err != nil {
			continue
		}
		doc, err := xmldom.Parse(part)
		if err != nil {
			continue
		}
		for _, s := range elements(doc.Root, aNS, "tblStyle") {
			style, _ := json.Marshal(readTableStyle(s, nil))
			if bytes.Contains(style, []byte(`"rgb"`)) {
				// a built-in style has theme colors: LibreOffice writes them resolved
				continue
			}
			id := s.Get("styleId")
			if seen[id] == nil {
				seen[id] = map[string]int{}
			}
			seen[id][string(style)]++
		}
	}
	common := func(counts map[string]int) string {
		keys := slices.Sorted(maps.Keys(counts))
		return slices.MaxFunc(keys, func(a, b string) int { return counts[a] - counts[b] })
	}
	accent := func(n int) string { return `"scheme":"accent` + strconv.Itoa(n) + `"` }

	out := map[string]json.RawMessage{}
	for _, f := range families {
		pair := f.name == "Dark Style 2"
		templates := map[string]int{}
		for n := 1; n <= 6; n++ {
			if s := seen[f.ids[n]]; s != nil {
				tmpl := strings.ReplaceAll(common(s), accent(n), `"scheme":"ACCENT"`)
				if pair {
					tmpl = strings.ReplaceAll(tmpl, accent(n+1), `"scheme":"ACCENT2"`)
				}
				templates[tmpl]++
			}
		}
		if len(templates) == 0 {
			t.Fatalf("%s: no style of the family in the corpus", f.name)
		}
		if len(templates) > 1 {
			t.Logf("%s: %d templates, the most common taken", f.name, len(templates))
		}
		tmpl := common(templates)
		for n, id := range f.ids {
			if id == "" {
				continue
			}
			var style string
			switch {
			case n == 0 && seen[id] != nil:
				style = common(seen[id])
			case n == 0:
				style = strings.ReplaceAll(strings.ReplaceAll(tmpl, `"ACCENT2"`, `"dk1"`), `"ACCENT"`, `"dk1"`)
			default:
				style = strings.ReplaceAll(strings.ReplaceAll(tmpl, `"ACCENT2"`, `"accent`+strconv.Itoa(n+1)+`"`), `"ACCENT"`, `"accent`+strconv.Itoa(n)+`"`)
				if s := seen[id]; s != nil && common(s) != style {
					t.Logf("%s accent %d: the corpus differs from the family", f.name, n)
				}
			}
			out[id] = json.RawMessage(style)
		}
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, id := range slices.Sorted(maps.Keys(out)) {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(strconv.Quote(id) + ":" + string(out[id]))
	}
	b.WriteString("\n}\n")
	data := b.Bytes()
	if *update {
		if err := os.WriteFile("tablestyles.json", data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old, err := os.ReadFile("tablestyles.json")
	if err != nil || !bytes.Equal(old, data) {
		t.Fatal("tablestyles.json is stale: go test ./pptx -run BuiltinStyles -update")
	}
}
