package opc

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmlcanon"
)

// ooxmlExtensions are the files the corpus tests open.
var ooxmlExtensions = []string{
	".docx", ".docm", ".dotx", ".dotm",
	".xlsx", ".xlsm", ".xltx", ".xltm",
	".pptx", ".pptm", ".potx", ".potm", ".ppsx", ".ppsm",
}

// corpus lists the OOXML files fetched by corpus/fetch.sh.
func corpus(t *testing.T) []string {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	t.Helper()
	root := filepath.Join("..", "corpus", "files")
	if _, err := os.Stat(root); err != nil {
		t.Skip("no corpus: run corpus/fetch.sh")
	}
	var files []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && slices.Contains(ooxmlExtensions, strings.ToLower(filepath.Ext(path))) {
			files = append(files, path)
		}
		return err
	})
	return files
}

// readRelationships reads and resolves every relationship of the package.
func readRelationships(p *Package) error {
	for _, name := range append([]string{""}, p.Names()...) {
		rels, err := p.Relationships(name)
		if err != nil {
			return err
		}
		for _, r := range rels {
			if _, err := Resolve(name, r.Target); !r.External && err != nil {
				return err
			}
		}
	}
	return nil
}

func TestCorpusRoundTrip(t *testing.T) {
	var opened, rejected int
	for _, path := range corpus(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		p, err := Open(data, Limits{})
		if err != nil {
			rejected++
			t.Logf("rejected %s: %v", filepath.Base(path), err)
			continue
		}
		if err := readRelationships(p); err != nil {
			rejected++
			t.Logf("rejected %s: %v", filepath.Base(path), err)
			continue
		}
		opened++
		out, err := p.Bytes()
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		q, err := Open(out, Limits{})
		if err != nil {
			t.Errorf("%s: written package does not open: %v", path, err)
			continue
		}
		if !slices.Equal(p.Names(), q.Names()) {
			t.Errorf("%s: parts changed", path)
			continue
		}
		if !slices.EqualFunc(rawEntries(t, data), rawEntries(t, out), bytes.Equal) {
			t.Errorf("%s: compressed data changed", path)
		}
	}
	t.Logf("%d packages kept intact, %d rejected", opened, rejected)
}

// TestCorpusContentTypesRewrite adds and removes a part, which rewrites
// [Content_Types].xml: the rewrite must mean what the original meant. When
// LOFFICE_OUT is set, the packages are written there for the validator.
func TestCorpusContentTypesRewrite(t *testing.T) {
	out := os.Getenv("LOFFICE_OUT")
	var rewrites, identical int
	root := filepath.Join("..", "corpus", "files")
	for _, path := range corpus(t) {
		data, _ := os.ReadFile(path)
		p, err := Open(data, Limits{})
		if err != nil || readRelationships(p) != nil {
			continue
		}
		original, err := p.Read(contentTypesName)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Add("bref-probe.bin", "application/x-bref-probe", nil); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if err := p.Remove("bref-probe.bin"); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		rewritten, err := p.Read(contentTypesName)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(original, rewritten) {
			identical++
		}
		rewrites++
		if err := xmlcanon.Diff(original, rewritten); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if out != "" {
			written, err := p.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			rel, _ := filepath.Rel(root, path)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, written, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d rewritten, %d identical to the byte", rewrites, identical)
}
