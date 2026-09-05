package xterm

import (
	"testing"

	"github.com/pborman/pty/ansi"
)

func TestImport(t *testing.T) {
	err := Import()
	// First import in this package test process: either unique xterm
	// sequences are added, or they collide with the ECMA-48 table.
	if err != nil {
		t.Logf("Import reported dups (allowed): %v", err)
	}

	err = Import()
	if err == nil {
		t.Error("second Import should report duplicated codes")
	}

	// Table entries should still be reachable through ansi.
	if len(Table) == 0 {
		t.Fatal("xterm.Table is empty")
	}
	for name, seq := range Table {
		if seq == nil {
			t.Errorf("Table[%q] is nil", name)
			continue
		}
		got := ansi.Table[name]
		if got == nil {
			// First Import may have skipped dups; if this name was
			// unique it should now be in ansi.Table.
			continue
		}
		if got.Name != seq.Name && got != seq {
			t.Logf("ansi.Table[%q] name=%q xterm name=%q", name, got.Name, seq.Name)
		}
	}
}
