package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "../../agent/testdata/sample.docx"

// session runs the command on the lines a client sends and gives the
// answers to its requests, by id, and the exit code.
func session(t *testing.T, args []string, requests ...string) (map[int]string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(strings.Join(requests, "\n")+"\n"), &stdout, &stderr)
	answers := map[int]string{}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var m struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		answers[m.ID] = line
	}
	if code != 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	return answers, code
}

func callLine(id int, tool string, args map[string]any) string {
	args["on_behalf_of"] = "dev"
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	return string(data)
}

func rootWithSample(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rapport.docx"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestServesTheRootAndSavesWhatIsEdited(t *testing.T) {
	dir := rootWithSample(t)
	before, _ := os.ReadFile(filepath.Join(dir, "rapport.docx"))
	answers, code := session(t, []string{"-root", dir, "-name", "Héraut"},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		callLine(2, "doc_list", map[string]any{}),
		callLine(3, "doc_read", map[string]any{"doc": "rapport.docx", "from": "p2", "to": "p2"}),
		callLine(4, "text_replace", map[string]any{"doc": "rapport.docx", "block": "p2", "find": "en avance", "text": "à l'heure"}),
	)
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(answers[2], "rapport.docx") {
		t.Errorf("doc_list: %s", answers[2])
	}
	if !strings.Contains(answers[3], "Le projet est **en avance**") {
		t.Errorf("doc_read: %s", answers[3])
	}
	if strings.Contains(answers[4], `"isError":true`) {
		t.Fatalf("text_replace: %s", answers[4])
	}
	after, _ := os.ReadFile(filepath.Join(dir, "rapport.docx"))
	if bytes.Equal(before, after) {
		t.Fatal("the edit was not saved when the input closed")
	}
	xml := part(t, after, "word/document.xml")
	if !strings.Contains(xml, "<w:ins ") || !strings.Contains(xml, `w:author="Héraut"`) || !strings.Contains(xml, "à l") {
		t.Errorf("saved document lacks the tracked change by Héraut")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

func part(t *testing.T, docx []byte, name string) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == name {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			data, _ := io.ReadAll(r)
			return string(data)
		}
	}
	t.Fatalf("no part %s", name)
	return ""
}

func TestRefusesEveryPathOutsideItsRoot(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(sample)
	secret := filepath.Join(outer, "secret.docx")
	if err := os.WriteFile(secret, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.docx"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"link.docx": secret, "dirlink": outer} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	keys := []string{
		"../secret.docx", "sub/../../secret.docx", secret, "/etc/passwd", "./inside.docx", "inside.docx/", `..\secret.docx`,
		"link.docx", "dirlink/secret.docx", ".", "", "inside.docx\x00",
	}
	for i, key := range keys {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			for _, tool := range []string{"doc_read", "text_replace"} {
				args := map[string]any{"doc": key, "block": "p2", "find": "en avance", "text": "x"}
				if key == "" {
					delete(args, "doc")
				}
				answers, code := session(t, []string{"-root", root}, callLine(i+1, tool, args))
				if code != 0 {
					t.Fatalf("exit code %d", code)
				}
				if !strings.Contains(answers[i+1], `"isError":true`) {
					t.Errorf("%s %q was served: %s", tool, key, answers[i+1])
				}
			}
		})
	}
	if got, _ := os.ReadFile(secret); !bytes.Equal(got, data) {
		t.Error("a document outside the root was written")
	}
	if got, _ := os.ReadFile(filepath.Join(root, "inside.docx")); !bytes.Equal(got, data) {
		t.Error("a refused call changed the document")
	}
	entries, _ := os.ReadDir(outer)
	if len(entries) != 2 {
		t.Errorf("files appeared outside the root: %v", entries)
	}
}

func TestDirStoreStaysUnderItsRoot(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../secret.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	store := dirStore{r}
	for _, key := range []string{"../secret.txt", "link.txt", "/etc/passwd", "a/../../secret.txt"} {
		if data, err := store.Load(context.Background(), key); err == nil {
			t.Errorf("Load(%q) gave %q", key, data)
		}
		if err := store.Save(context.Background(), key, []byte("x")); err == nil {
			t.Errorf("Save(%q) saved", key)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(outer, "secret.txt")); string(got) != "secret" {
		t.Errorf("secret became %q", got)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"-root", ""}, {"-root", t.TempDir(), "extra"}, {"-bogus"}, {"-root", filepath.Join(t.TempDir(), "absent")}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, strings.NewReader(""), &stdout, &stderr); code == 0 {
			t.Errorf("%v: exit code 0", args)
		}
		if stdout.Len() > 0 {
			t.Errorf("%v: wrote on standard output: %s", args, stdout.String())
		}
	}
}

func TestListSkipsWhatIsNotADocument(t *testing.T) {
	dir := rootWithSample(t)
	for name, content := range map[string]string{"notes.md": "x", ".hidden/a.docx": "x", "sub/b.docx": "x", ".c.docx.1.tmp": "x"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	answers, _ := session(t, []string{"-root", dir}, callLine(1, "doc_list", map[string]any{}))
	got := answers[1]
	for _, want := range []string{"rapport.docx", "sub/b.docx"} {
		if !strings.Contains(got, want) {
			t.Errorf("doc_list lacks %s: %s", want, got)
		}
	}
	for _, not := range []string{"notes.md", "hidden", "tmp"} {
		if strings.Contains(got, not) {
			t.Errorf("doc_list has %s: %s", not, got)
		}
	}
}
