// Package compat embeds a source-pinned Manim API inventory. Entries are marked
// verified only after a corresponding behavioral parity test has passed.
package compat

import (
	_ "embed"
	"encoding/json"
)

//go:embed manim-api.json
var inventory []byte

type Source struct{ Version, Commit string }
type Symbol struct {
	Module, Name, Kind, Status string
	Line                       int
}
type Ledger struct {
	Source  Source
	Modules []string
	Symbols []Symbol
}

func Load() (*Ledger, error) {
	var l Ledger
	if e := json.Unmarshal(inventory, &l); e != nil {
		return nil, e
	}
	return &l, nil
}
