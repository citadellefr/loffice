package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/citadellefr/loffice/xlsx"
)

func init() {
	if len(os.Args) > 2 && os.Args[1] == "dump" {
		data, _ := os.ReadFile(os.Args[2])
		_, tree, err := xlsx.Open(data, xlsx.Limits{})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		e := tree.Edit()
		for _, c := range e[:min(len(e), 12)] {
			b, _ := json.Marshal(c)
			fmt.Println(string(b)[:min(len(b), 300)])
		}
		os.Exit(0)
	}
}
