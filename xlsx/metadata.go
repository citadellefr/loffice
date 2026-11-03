package xlsx

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

const (
	relSheetMetadata  = relBase + "sheetMetadata"
	typeSheetMetadata = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheetMetadata+xml"
	dynamicArrayNS    = "http://schemas.microsoft.com/office/spreadsheetml/2017/dynamicarray"

	metadataType   = `<metadataType name="XLDAPR" minSupportedVersion="120000" copy="1" pasteAll="1" pasteValues="1" merge="1" splitFirst="1" rowColShift="1" clearFormats="1" clearComments="1" assign="1" coerce="1" cellMeta="1"/>`
	futureMetadata = `<futureMetadata name="XLDAPR" count="1"><bk><extLst><ext uri="{bdbb8cdc-fa1e-496e-a857-3c3f30c029c3}"><xda:dynamicArrayProperties fDynamic="1" fCollapsed="0"/></ext></extLst></bk></futureMetadata>`
)

// metadataOrder is the sequence of the children of metadata.
var metadataOrder = []string{"metadataTypes", "metadataStrings", "mdxMetadata", "futureMetadata", "cellMetadata", "valueMetadata", "extLst"}

// metadata makes sure the workbook describes the dynamic arrays its cells
// point to with cm="1": the first block of cell metadata.
func (w *writer) metadata() error {
	name := target(w.d.bookName, w.rels, relSheetMetadata)
	if name == "" || !w.pkg.Has(name) {
		name = "xl/metadata.xml"
		if w.pkg.Has(name) {
			name = w.freeName("xl/metadata%d.xml")
		}
		w.addRel(relSheetMetadata, w.target(name))
		return w.put(name, typeSheetMetadata, []byte(xmlHeader+`<metadata xmlns="`+mainNS+`" xmlns:xda="`+dynamicArrayNS+`">`+
			`<metadataTypes count="1">`+metadataType+`</metadataTypes>`+futureMetadata+
			`<cellMetadata count="1"><bk><rc t="1" v="0"/></bk></cellMetadata></metadata>`))
	}
	data, err := w.pkg.Read(name)
	if err != nil {
		return err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	root := doc.Root
	if root.Child(mainNS, "cellMetadata") != nil {
		return nil
	}
	types := root.Child(mainNS, "metadataTypes")
	if types == nil {
		types = xmldom.New(mainNS, "metadataTypes")
		root.Insert(types, metadataOrder)
	}
	spaces := map[string]string{"": mainNS, "xda": dynamicArrayNS}
	for _, s := range []string{metadataType, futureMetadata, `<cellMetadata count="1"><bk><rc t="` + strconv.Itoa(len(types.Elements())+1) + `" v="0"/></bk></cellMetadata>`} {
		e, err := xmldom.ParseFragment([]byte(s), spaces)
		if err != nil {
			return err
		}
		if e.Local == "metadataType" {
			types.Append(e)
			types.Set("count", strconv.Itoa(len(types.Elements())))
			continue
		}
		root.Insert(e, metadataOrder)
	}
	if _, ok := root.Attr("xmlns:xda"); !ok {
		root.Set("xmlns:xda", dynamicArrayNS)
	}
	return w.pkg.Set(name, doc.Bytes())
}
