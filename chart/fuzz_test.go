package chart

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func FuzzRead(f *testing.F) {
	paths, _ := filepath.Glob("testdata/*.xml")
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := Read(data)
		if err != nil {
			return
		}
		if !json.Valid(c.JSON()) {
			t.Fatal("chart does not give JSON")
		}
	})
}
