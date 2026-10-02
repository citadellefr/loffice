package pagination

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fonts are the twin fonts, found under the directories of $LOFFICE_FONTS.
func fonts(t testing.TB) *FontSet {
	fs := NewFontSet(filepath.SplitList(os.Getenv("LOFFICE_FONTS"))...)
	if !fs.Complete() {
		t.Skip("set LOFFICE_FONTS to directories holding Carlito, Caladea and Liberation")
	}
	return fs
}

type measured struct {
	name        string
	word, ours  int
	flow, kept  int
	unsupported []string
	elapsed     time.Duration
}

func measure(t *testing.T, o Options) []measured {
	fs := fonts(t)
	paths, _ := filepath.Glob(filepath.Join("..", "..", "..", "corpus", "files", "*", "*.docx"))
	if len(paths) == 0 {
		t.Skip("no corpus: run corpus/fetch.sh")
	}
	var out []measured
	for _, path := range paths {
		data, _ := os.ReadFile(path)
		d, err := Load(data, fs)
		if err != nil || !d.FromWord || d.WordPages == 0 || anonymized(d) {
			continue
		}
		start := time.Now()
		r := Paginate(d, o)
		elapsed := time.Since(start)
		o.Sync = true
		synced := Paginate(d, o)
		o.Sync = false
		m := measured{name: filepath.Base(path), word: d.WordPages, ours: r.Pages,
			flow: synced.Flow, kept: synced.Kept, elapsed: elapsed}
		for u := range d.Unsupported {
			m.unsupported = append(m.unsupported, u)
		}
		sort.Strings(m.unsupported)
		out = append(out, m)
	}
	return out
}

func summary(ms []measured) string {
	var docs, same, multi, multiSame, flow, kept, cleanFlow, cleanKept int
	for _, m := range ms {
		docs++
		if m.word == m.ours {
			same++
		}
		if m.word > 1 {
			multi++
			if m.word == m.ours {
				multiSame++
			}
		}
		flow += m.flow
		kept += m.kept
		if len(m.unsupported) == 0 {
			cleanFlow += m.flow
			cleanKept += m.kept
		}
	}
	return fmt.Sprintf("page count %d/%d (multi-page %d/%d); pages ending where Word's do: %d/%d, %d/%d without unsupported features",
		same, docs, multiSame, multi, kept, flow, cleanKept, cleanFlow)
}

// TestAgainstWord lays out the Word documents of the corpus and compares
// with what Word recorded: the page count, and page by page, whether a page
// started where Word started it ends where Word ended it.
func TestAgainstWord(t *testing.T) {
	ms := measure(t, DefaultOptions)
	for _, m := range ms {
		if m.word == 1 && m.ours == 1 && m.flow == 0 {
			continue
		}
		mark := " "
		if m.word != m.ours || m.kept != m.flow {
			mark = "≠"
		}
		t.Logf("%s %-50.50s Word %3d ours %3d  pages kept %2d/%2d  %8v  %s",
			mark, m.name, m.word, m.ours, m.kept, m.flow, m.elapsed.Round(time.Microsecond), strings.Join(m.unsupported, ", "))
	}
	t.Log(summary(ms))

	// which unsupported features cost the most pages
	type tally struct{ docs, flow, kept int }
	byFeature := map[string]*tally{}
	for _, m := range ms {
		seen := map[string]bool{}
		for _, u := range m.unsupported {
			if strings.HasPrefix(u, "font ") {
				u = "fonts without a twin"
			}
			if seen[u] {
				continue
			}
			seen[u] = true
			if byFeature[u] == nil {
				byFeature[u] = &tally{}
			}
			byFeature[u].docs++
			byFeature[u].flow += m.flow
			byFeature[u].kept += m.kept
		}
	}
	for u, c := range byFeature {
		if c.flow > 0 {
			t.Logf("with %-22s %4d documents, pages kept %3d/%3d", u+":", c.docs, c.kept, c.flow)
		}
	}
}

func BenchmarkPaginate(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "corpus", "files", "poi", "bug65649.docx"))
	if err != nil {
		b.Skip("no corpus: run corpus/fetch.sh")
	}
	d, err := Load(data, fonts(b))
	if err != nil {
		b.Fatal(err)
	}
	pages := Paginate(d, DefaultOptions).Pages
	for b.Loop() {
		Paginate(d, DefaultOptions)
	}
	b.ReportMetric(float64(pages), "pages")
}

// TestOptions compares the rules whose Word behavior is uncertain.
func TestOptions(t *testing.T) {
	for _, o := range []Options{
		DefaultOptions,
		{SuppressBeforeAtTop: false, Borders: true},
		{SuppressBeforeAtTop: true, Borders: false},
		{SuppressBeforeAtTop: true, Borders: true, Round: 0.05},
		{SuppressBeforeAtTop: true, Borders: true, Round: 0.75},
	} {
		t.Logf("%+v: %s", o, summary(measure(t, o)))
	}
}

// TestDump shows, for the document named by $LOFFICE_DOC, where each page
// starts in our layout and in Word's.
func TestDump(t *testing.T) {
	name := os.Getenv("LOFFICE_DOC")
	if name == "" {
		t.Skip("set LOFFICE_DOC to a corpus file name")
	}
	paths, _ := filepath.Glob(filepath.Join("..", "..", "..", "corpus", "files", "*", name))
	if len(paths) == 0 {
		t.Fatal("not found")
	}
	data, _ := os.ReadFile(paths[0])
	d, err := Load(data, fonts(t))
	if err != nil {
		t.Fatal(err)
	}
	r := Paginate(d, DefaultOptions)
	text := map[int]string{}
	word := map[int][]int{}
	var walk func([]Block, string)
	walk = func(blocks []Block, in string) {
		for _, b := range blocks {
			switch b := b.(type) {
			case *Para:
				var s strings.Builder
				for _, it := range b.Items {
					if it.Kind == TextItem {
						s.WriteString(it.Text)
					}
				}
				text[b.Index] = in + s.String()
				word[b.Index] = b.WordBreaks
			case *Table:
				for _, row := range b.Rows {
					for _, c := range row.Cells {
						walk(c.Blocks, "[cell] ")
					}
				}
			}
		}
	}
	walk(d.Body, "")
	ours := map[int][]int{}
	for _, b := range r.Breaks {
		ours[b.Para] = append(ours[b.Para], b.Offset)
	}
	for i := range d.Paragraphs {
		if len(word[i]) > 0 || len(ours[i]) > 0 {
			t.Logf("para %4d Word %v ours %v  %.60q", i, word[i], ours[i], text[i])
		}
	}
	t.Logf("Word %d pages, ours %d", d.WordPages, r.Pages)
}
