package opc

import (
	"errors"
	"hash/crc32"
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16Bytes(s string, bigEndian bool) []byte {
	out := []byte{0xFF, 0xFE}
	if bigEndian {
		out = []byte{0xFE, 0xFF}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func TestEncodedParts(t *testing.T) {
	for name, c := range map[string]struct{ data, want string }{
		"utf-8":           {`<?xml version="1.0" encoding="UTF-8"?><a>é</a>`, `<?xml version="1.0" encoding="UTF-8"?><a>é</a>`},
		"no declaration":  {`<a>é</a>`, `<a>é</a>`},
		"no encoding":     {`<?xml version="1.0"?><a>é</a>`, `<?xml version="1.0"?><a>é</a>`},
		"latin-1":         {"<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><a>S\xe9rie \xa0</a>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><a>Série  </a>"},
		"single quotes":   {"<?xml version='1.0' encoding = 'iso-8859-1' ?><a>\xe9</a>", "<?xml version='1.0' encoding = 'UTF-8' ?><a>é</a>"},
		"windows-1252":    {"<?xml version=\"1.0\" encoding=\"windows-1252\"?><a>\x80 \x93x\x94</a>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><a>€ “x”</a>"},
		"utf-16le":        {string(utf16Bytes(`<?xml version="1.0" encoding="UTF-16"?><a>é😀</a>`, false)), `<?xml version="1.0" encoding="UTF-8"?><a>é😀</a>`},
		"utf-16be":        {string(utf16Bytes(`<?xml version="1.0" encoding="utf-16"?><a>é</a>`, true)), `<?xml version="1.0" encoding="UTF-8"?><a>é</a>`},
		"utf-16 only bom": {string(utf16Bytes(`<a>é</a>`, false)), `<a>é</a>`},
	} {
		p, err := Open(makeZip(t, entry{contentTypesName, minimalTypes}, entry{"a.xml", c.data}), Limits{})
		if err != nil {
			t.Fatal(name, err)
		}
		got, err := p.Read("a.xml")
		if err != nil || string(got) != c.want {
			t.Errorf("%s: %q, %v, want %q", name, got, err, c.want)
		}
	}
}

func TestEncodedPartsKeepTheirBytes(t *testing.T) {
	part := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><a>\xe9</a>"
	p, err := Open(makeZip(t, entry{contentTypesName, minimalTypes}, entry{"a.xml", part}), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read("a.xml"); err != nil {
		t.Fatal(err)
	}
	p = reopen(t, p)
	if crc, _, ok := p.Checksum("a.xml"); !ok || crc != crc32.ChecksumIEEE([]byte(part)) {
		t.Fatal("an untouched part was rewritten")
	}
}

func TestUnknownEncoding(t *testing.T) {
	p, err := Open(makeZip(t, entry{contentTypesName, minimalTypes}, entry{"a.xml", `<?xml version="1.0" encoding="EBCDIC"?><a/>`}), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Read("a.xml")
	if !errors.Is(err, ErrInvalid) || err.Error() != `opc: invalid package: a.xml: encoding "EBCDIC"` {
		t.Fatal(err)
	}
}

func TestContentTypesInLatin1(t *testing.T) {
	types := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" + strings.TrimPrefix(minimalTypes, `<?xml version="1.0"?>`)
	if _, err := Open(makeZip(t, entry{contentTypesName, types}), Limits{}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenMessages(t *testing.T) {
	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"no content types": {makeZip(t, entry{"a.xml", "<a/>"}), "opc: invalid package: [Content_Types].xml is missing"},
		"OpenDocument": {
			makeZip(t, entry{"mimetype", "application/vnd.oasis.opendocument.text"}, entry{"content.xml", "<a/>"}),
			"opc: invalid package: an OpenDocument file, not Open XML",
		},
		"duplicate part": {
			makeZip(t, entry{contentTypesName, minimalTypes}, entry{"a.xml", ""}, entry{"a.xml", ""}),
			`opc: invalid package: duplicate part "a.xml"`,
		},
		"entities in the content types": {
			makeZip(t, entry{contentTypesName, `<?xml version="1.0"?><!DOCTYPE Types [<!ENTITY a "b">]>` + strings.TrimPrefix(minimalTypes, `<?xml version="1.0"?>`)}),
			"opc: invalid package: [Content_Types].xml: xmltok: syntax error at offset 55: entity declarations are not allowed",
		},
	} {
		if _, err := Open(c.data, Limits{}); err == nil || err.Error() != c.want {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestDoctypeWithoutEntities(t *testing.T) {
	types := `<?xml version="1.0"?><!DOCTYPE Types>` + strings.TrimPrefix(minimalTypes, `<?xml version="1.0"?>`)
	rels := `<?xml version="1.0"?><!DOCTYPE Relationships [<!ELEMENT Relationships ANY>]><Relationships xmlns="` + relsNamespace + `"/>`
	p, err := Open(makeZip(t, entry{contentTypesName, types}, entry{"_rels/.rels", rels}), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Relationships(""); err != nil {
		t.Fatal(err)
	}
	rels = strings.Replace(rels, "<!ELEMENT Relationships ANY>", `<!ENTITY a SYSTEM "file:///etc/passwd">`, 1)
	p, err = Open(makeZip(t, entry{contentTypesName, types}, entry{"_rels/.rels", rels}), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Relationships(""); err == nil || !strings.HasSuffix(err.Error(), "entity declarations are not allowed") {
		t.Fatal(err)
	}
}
