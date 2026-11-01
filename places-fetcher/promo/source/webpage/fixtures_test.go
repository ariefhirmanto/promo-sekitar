package webpage

import (
	"os"
	"path/filepath"
	"testing"
)

// Test pages live in testdata/webpage/ as hand-written HTML: the markup each host rule reads,
// around an invented brand ("Mie Contoh") and invented text, never a copy of a live page (rule 12
// in .claude/rules/adapters.md).
const fixtures = "../../../testdata/webpage"

func fx(name string) string { return filepath.Join(fixtures, name) }

// page writes html to a temp file for fileFetcher (the BCA pages are built from script chunks).
func page(t *testing.T, html string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
