// Package opc reads and writes Open Packaging Conventions packages, the zip
// containers of .docx, .xlsx and .pptx files.
//
// A package is opened from bytes, its parts are read on demand, and writing
// it back copies every part the caller did not replace byte for byte,
// compressed data included: what the caller does not understand is never lost.
package opc

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// Limits bound what Open accepts, against zip bombs. Zero fields take the
// defaults, which suit a small server: the largest package of the test
// corpus has 1714 parts, a part of 46 MiB and 46 MiB in all.
type Limits struct {
	// MaxParts is the number of parts, 4096 by default.
	MaxParts int
	// MaxPartSize and MaxTotalSize bound uncompressed sizes, 64 MiB and
	// 128 MiB by default. The sizes an archive declares are enforced while
	// reading, so it cannot lie.
	MaxPartSize  int64
	MaxTotalSize int64
}

func (l Limits) withDefaults() Limits {
	if l.MaxParts <= 0 {
		l.MaxParts = 4096
	}
	if l.MaxPartSize <= 0 {
		l.MaxPartSize = 64 << 20
	}
	if l.MaxTotalSize <= 0 {
		l.MaxTotalSize = 128 << 20
	}
	return l
}

var (
	ErrInvalid  = errors.New("opc: invalid package")
	ErrTooLarge = errors.New("opc: package too large")
	ErrNotFound = errors.New("opc: part not found")
	// ErrCompound is an OLE compound file rather than a zip: a document
	// encrypted with a password, or a legacy binary format under an OOXML name.
	ErrCompound = errors.New("opc: compound file, encrypted or legacy binary")
)

var compoundSignature = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

const contentTypesName = "[Content_Types].xml"

// Package is an open package. It is not safe for concurrent use.
type Package struct {
	parts   []*part
	index   map[string]*part
	types   *contentTypes
	comment string
	strict  bool
}

type part struct {
	name string
	// file is the original entry, nil once the part was replaced or for a
	// part added since Open.
	file *zip.File
	data []byte
	// method and modified are kept from the original entry when the part is
	// replaced.
	method   uint16
	modified time.Time
}

// Open reads the directory of a package and its content types. Part data is
// only decompressed when read.
func Open(data []byte, limits Limits) (*Package, error) {
	limits = limits.withDefaults()
	if bytes.HasPrefix(data, compoundSignature) {
		return nil, ErrCompound
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	p := &Package{index: map[string]*part{}, comment: zr.Comment}
	var total int64
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		// some writers use backslashes, which Office accepts
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if !validName(name) {
			return nil, fmt.Errorf("%w: part name %q", ErrInvalid, f.Name)
		}
		key := strings.ToLower(name)
		if p.index[key] != nil {
			return nil, fmt.Errorf("%w: duplicate part %q", ErrInvalid, name)
		}
		if len(p.parts) == limits.MaxParts {
			return nil, fmt.Errorf("%w: more than %d parts", ErrTooLarge, limits.MaxParts)
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, fmt.Errorf("%w: part %q: compression method %d", ErrInvalid, f.Name, f.Method)
		}
		if _, err := f.DataOffset(); err != nil {
			return nil, fmt.Errorf("%w: part %q: %v", ErrInvalid, f.Name, err)
		}
		size := f.UncompressedSize64
		if size > uint64(limits.MaxPartSize) {
			return nil, fmt.Errorf("%w: part %q is %d bytes", ErrTooLarge, f.Name, size)
		}
		if total += int64(size); total > limits.MaxTotalSize {
			return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, limits.MaxTotalSize)
		}
		pt := &part{name: name, file: f, method: f.Method, modified: f.Modified}
		p.parts = append(p.parts, pt)
		p.index[key] = pt
	}
	raw, err := p.read(contentTypesName)
	if errors.Is(err, ErrNotFound) {
		if mime, err := p.read("mimetype"); err == nil && bytes.HasPrefix(mime, []byte("application/vnd.oasis.opendocument.")) {
			return nil, fmt.Errorf("%w: an OpenDocument file, not Open XML", ErrInvalid)
		}
		return nil, fmt.Errorf("%w: %s is missing", ErrInvalid, contentTypesName)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if raw, err = utf8XML(contentTypesName, raw); err != nil {
		return nil, err
	}
	if p.types, err = parseContentTypes(raw); err != nil {
		return nil, err
	}
	// A renamed entry also takes the spelling of its override, for readers
	// that compare part names case-sensitively.
	for _, pt := range p.parts {
		if i := p.types.find(pt.name); i >= 0 && pt.name != pt.file.Name {
			pt.name = p.types.Rules[i].PartName[1:]
		}
	}
	p.strict = p.isStrict()
	return p, nil
}

// validName accepts the zip names of OPC parts: relative, forward slashes,
// no empty, "." or ".." segment.
func validName(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\\x00") || path.Clean("/"+name) != "/"+name {
		return false
	}
	for seg := range strings.SplitSeq(name, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// Names lists the parts in archive order, [Content_Types].xml included.
func (p *Package) Names() []string {
	names := make([]string, len(p.parts))
	for i, pt := range p.parts {
		names[i] = pt.name
	}
	return names
}

// Has reports whether the package holds the part; names compare
// case-insensitively, as OPC requires.
func (p *Package) Has(name string) bool {
	return p.index[strings.ToLower(name)] != nil
}

// Read returns the uncompressed content of a part, as Transitional in a
// Strict package.
func (p *Package) Read(name string) ([]byte, error) {
	data, err := p.read(name)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(p.ContentType(name), "xml") {
		if data, err = utf8XML(name, data); err != nil {
			return nil, err
		}
	}
	return p.translate(p.ContentType(name), data, toTransitional), nil
}

func (p *Package) read(name string) ([]byte, error) {
	pt := p.index[strings.ToLower(name)]
	if pt == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if pt.file == nil {
		return pt.data, nil
	}
	rc, err := pt.file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	buf.Grow(int(pt.file.UncompressedSize64))
	if _, err := buf.ReadFrom(rc); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
	}
	return buf.Bytes(), nil
}

// Checksum is the CRC-32 and size of a part as the archive records them,
// known without reading the part; false for a part written since Open.
func (p *Package) Checksum(name string) (crc uint32, size uint64, ok bool) {
	pt := p.index[strings.ToLower(name)]
	if pt == nil || pt.file == nil {
		return 0, 0, false
	}
	return pt.file.CRC32, pt.file.UncompressedSize64, true
}

// ContentType is the media type of a part, "" when the package declares none.
func (p *Package) ContentType(name string) string {
	return p.types.lookup(name)
}

// Set replaces the content of an existing part, written back to Strict in
// a Strict package.
func (p *Package) Set(name string, data []byte) error {
	pt := p.index[strings.ToLower(name)]
	if pt == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	pt.file, pt.data = nil, p.translate(p.ContentType(name), data, toStrict)
	return nil
}

// Add creates a part, declaring its content type unless the extension's
// default already matches.
func (p *Package) Add(name, contentType string, data []byte) error {
	if !validName(name) || strings.EqualFold(name, contentTypesName) {
		return fmt.Errorf("%w: part name %q", ErrInvalid, name)
	}
	key := strings.ToLower(name)
	if p.index[key] != nil {
		return fmt.Errorf("%w: part %q already exists", ErrInvalid, name)
	}
	pt := &part{name: name, data: p.translate(contentType, data, toStrict), method: zip.Deflate, modified: zipEpoch}
	p.parts = append(p.parts, pt)
	p.index[key] = pt
	if p.types.lookup(name) != contentType {
		p.types.override(name, contentType)
		return p.Set(contentTypesName, p.types.marshal())
	}
	return nil
}

// Remove deletes a part and its content type override, if any.
func (p *Package) Remove(name string) error {
	key := strings.ToLower(name)
	pt := p.index[key]
	if pt == nil || key == strings.ToLower(contentTypesName) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(p.index, key)
	for i, q := range p.parts {
		if q == pt {
			p.parts = append(p.parts[:i], p.parts[i+1:]...)
			break
		}
	}
	if p.types.removeOverride(name) {
		return p.Set(contentTypesName, p.types.marshal())
	}
	return nil
}

// zipEpoch is the timestamp Office gives every entry.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Write writes the package: parts in their original order, those never
// replaced copied without being decompressed, new ones appended.
func (p *Package) Write(w io.Writer) error {
	zw := zip.NewWriter(w)
	for _, pt := range p.parts {
		if pt.file != nil {
			if err := copyEntry(zw, pt); err != nil {
				return err
			}
			continue
		}
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: pt.name, Method: pt.method, Modified: pt.modified})
		if err != nil {
			return err
		}
		if _, err := fw.Write(pt.data); err != nil {
			return err
		}
	}
	if err := zw.SetComment(p.comment); err != nil {
		return err
	}
	return zw.Close()
}

// copyEntry writes an original entry without decompressing it, under its
// normalized name.
func copyEntry(zw *zip.Writer, pt *part) error {
	if pt.name == pt.file.Name {
		return zw.Copy(pt.file)
	}
	r, err := pt.file.OpenRaw()
	if err != nil {
		return err
	}
	h := pt.file.FileHeader
	h.Name = pt.name
	w, err := zw.CreateRaw(&h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

// Bytes is Write into memory.
func (p *Package) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
