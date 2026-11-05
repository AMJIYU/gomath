package compat

import "testing"

func TestSourcePinnedInventory(t *testing.T) {
	l, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if l.Source.Version != "0.22.1" || len(l.Source.Commit) != 40 || len(l.Modules) != 177 || len(l.Symbols) < 2000 {
		t.Fatal("incomplete source inventory")
	}
	for _, s := range l.Symbols {
		if s.Status != "unverified" {
			t.Fatal("compatibility must not be implied by API resemblance")
		}
	}
}
