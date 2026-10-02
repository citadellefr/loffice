package docx

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// builtinStyles are the styles of Word's blank document that files often
// leave out, written into styles.xml when a paragraph takes one: the
// editor offers them whatever the file defines.
var builtinStyles = []struct{ id, xml string }{
	{"Title", `<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="10"/><w:qFormat/><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/><w:contextualSpacing/></w:pPr><w:rPr><w:rFonts w:asciiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia" w:hAnsiTheme="majorHAnsi" w:cstheme="majorBidi"/><w:spacing w:val="-10"/><w:kern w:val="28"/><w:sz w:val="56"/><w:szCs w:val="56"/></w:rPr></w:style>`},
	{"Subtitle", `<w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="11"/><w:qFormat/><w:rPr><w:color w:val="5A5A5A"/><w:spacing w:val="15"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style>`},
	{"Heading1", `<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="9"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="240" w:after="0"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:rFonts w:asciiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia" w:hAnsiTheme="majorHAnsi" w:cstheme="majorBidi"/><w:color w:val="2F5496"/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style>`},
	{"Heading2", `<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="9"/><w:unhideWhenUsed/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="40" w:after="0"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:rFonts w:asciiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia" w:hAnsiTheme="majorHAnsi" w:cstheme="majorBidi"/><w:color w:val="2F5496"/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>`},
	{"Heading3", `<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="9"/><w:unhideWhenUsed/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="40" w:after="0"/><w:outlineLvl w:val="2"/></w:pPr><w:rPr><w:rFonts w:asciiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia" w:hAnsiTheme="majorHAnsi" w:cstheme="majorBidi"/><w:color w:val="1F3763"/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr></w:style>`},
	{"Quote", `<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="29"/><w:qFormat/><w:pPr><w:spacing w:before="200" w:after="160"/><w:ind w:left="864" w:right="864"/><w:jc w:val="center"/></w:pPr><w:rPr><w:i/><w:iCs/><w:color w:val="404040"/></w:rPr></w:style>`},
	{"ListParagraph", `<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:uiPriority w:val="34"/><w:qFormat/><w:pPr><w:ind w:left="720"/><w:contextualSpacing/></w:pPr></w:style>`},
	{"NoSpacing", `<w:style w:type="paragraph" w:styleId="NoSpacing"><w:name w:val="No Spacing"/><w:uiPriority w:val="1"/><w:qFormat/><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:style>`},
}

// builtinLists are the lists the editor starts when the document has none
// of their kind, by the name paragraphs give them before they are
// written: numbering.xml gets them, under numbers of its own.
var builtinLists = map[string]string{
	"bullet":  `<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>` + levels(true) + `</w:abstractNum>`,
	"decimal": `<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>` + levels(false) + `</w:abstractNum>`,
}

// levels are the nine levels of Word's default bullets or numbers.
func levels(bullet bool) string {
	var out string
	for i := range 9 {
		left := strconv.Itoa(720 * (i + 1))
		lvl := `<w:lvl w:ilvl="` + strconv.Itoa(i) + `">`
		if bullet {
			char, font := "\uf0b7", "Symbol"
			switch i % 3 {
			case 1:
				char, font = "o", "Courier New"
			case 2:
				char, font = "\uf0a7", "Wingdings"
			}
			lvl += `<w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="` + char + `"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="` + left + `" w:hanging="360"/></w:pPr><w:rPr><w:rFonts w:ascii="` + font + `" w:hAnsi="` + font + `" w:hint="default"/></w:rPr>`
		} else {
			format := [3]string{"decimal", "lowerLetter", "lowerRoman"}[i%3]
			jc := "left"
			hanging := "360"
			if i%3 == 2 {
				jc, hanging = "right", "180"
			}
			lvl += `<w:start w:val="1"/><w:numFmt w:val="` + format + `"/><w:lvlText w:val="%` + strconv.Itoa(i+1) + `."/><w:lvlJc w:val="` + jc + `"/><w:pPr><w:ind w:left="` + left + `" w:hanging="` + hanging + `"/></w:pPr>`
		}
		out += lvl + `</w:lvl>`
	}
	return out
}

// builtin parses a builtin element, in the main namespace under "w".
func builtin(xml string) *xmldom.Element {
	e, err := xmldom.ParseFragment([]byte(xml), map[string]string{"w": NS})
	if err != nil {
		panic(err)
	}
	return e
}
