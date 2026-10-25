// Command corpus downloads the test corpus into corpus/files, from pinned
// commits of projects whose test files are published under a permissive
// license.
//
//	go run ./tools/corpus
package main

import (
	"cmp"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const dest = "corpus/files"

type source struct {
	name, url, commit string
	paths             []string
}

var sources = []source{
	// Apache License 2.0
	{"poi", "https://github.com/apache/poi.git", "942d95d85b15d0dfdb3bc9ba1b4f273f277757c8",
		[]string{"/test-data/document/*", "/test-data/spreadsheet/*", "/test-data/slideshow/*"}},
	// MIT
	{"python-docx", "https://github.com/python-openxml/python-docx.git", "e45454602b53e8e572b179ccf1c91093ec9f4ed7",
		[]string{"*.docx"}},
	// Mozilla Public License 2.0: documents from the bug reports of Writer,
	// Impress and Calc
	{"libreoffice", "https://github.com/LibreOffice/core.git", "2a6ccbc40b060848a6eba2b2d27db43a625ded9a",
		[]string{"/sw/qa/**/*.docx", "/sd/qa/**/*.pptx", "/sd/qa/**/*.pptm", "/sd/qa/**/*.potx", "/sd/qa/**/*.ppsx",
			"/sc/qa/**/*.xlsx", "/sc/qa/**/*.xlsm", "/sc/qa/**/*.csv"}},
	// MIT
	{"python-pptx", "https://github.com/scanny/python-pptx.git", "278b47b1dedd5b46ee84c286e77cdfb0bf4594be",
		[]string{"*.pptx", "*.pptm", "*.xlsx"}},
}

func main() {
	log.SetFlags(0)
	tmp, err := os.MkdirTemp("", "corpus")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	if err := os.RemoveAll(dest); err != nil {
		log.Fatal(err)
	}
	for _, s := range sources {
		if err := s.fetch(tmp); err != nil {
			os.RemoveAll(tmp)
			log.Fatalf("%s: %v", s.name, err)
		}
	}
	summary()
}

func (s source) fetch(tmp string) error {
	fmt.Printf("%s@%.12s\n", s.name, s.commit)
	repo := filepath.Join(tmp, s.name)
	steps := [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", s.url},
		append([]string{"-C", repo, "sparse-checkout", "set", "--no-cone"}, s.paths...),
		{"-C", repo, "fetch", "-q", "--depth", "1", "--filter=blob:none", "origin", s.commit},
		{"-C", repo, "checkout", "-q", "FETCH_HEAD"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s: %w", args[0], err)
		}
	}
	return flatten(repo, filepath.Join(dest, s.name))
}

// flatten copies the files of repo into dir, a name met twice taking its
// directory as prefix.
func flatten(repo, dir string) error {
	var files []string
	err := filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.Type().IsRegular():
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(files)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		out := filepath.Join(dir, filepath.Base(f))
		if _, err := os.Lstat(out); err == nil {
			out = filepath.Join(dir, filepath.Base(filepath.Dir(f))+"-"+filepath.Base(f))
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// summary prints the most frequent extensions of the corpus.
func summary() {
	count := map[string]int{}
	filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			count[strings.TrimPrefix(filepath.Ext(path), ".")]++
		}
		return nil
	})
	type entry struct {
		ext string
		n   int
	}
	var list []entry
	for ext, n := range count {
		list = append(list, entry{ext, n})
	}
	slices.SortFunc(list, func(a, b entry) int { return cmp.Or(cmp.Compare(b.n, a.n), cmp.Compare(a.ext, b.ext)) })
	for _, e := range list[:min(len(list), 12)] {
		fmt.Printf("%7d %s\n", e.n, e.ext)
	}
}
