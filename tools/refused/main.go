package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/pptx"
	"github.com/citadellefr/loffice/xlsx"
)

func main() {
	files, _ := filepath.Glob("corpus/files/*/*")
	for _, f := range files {
		data, _ := os.ReadFile(f)
		var err error
		switch strings.ToLower(filepath.Ext(f)) {
		case ".docx", ".docm", ".dotx":
			_, _, err = docx.Open(data, opc.Limits{})
		case ".xlsx", ".xlsm":
			_, _, err = xlsx.Open(data, xlsx.Limits{})
		case ".pptx", ".pptm", ".potx", ".ppsx":
			_, _, err = pptx.Open(data, opc.Limits{})
		}
		if err != nil {
			fmt.Printf("%s\t%v\n", f, err)
		}
	}
}
