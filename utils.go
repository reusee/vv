package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/reusee/e"
)

var (
	me, we, ce, he = e.New(e.WithPackage("ovi"))
	pt             = fmt.Printf
)

func toJSON(v interface{}) string {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "    ")
	ce(encoder.Encode(v))
	return b.String()
}
