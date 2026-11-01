package geo

import "testing"

// Same point as the example row in the sheet's outlets tab.
func TestCellsMatchSheetExample(t *testing.T) {
	r8, r6 := NewH3Indexer().Cells(-6.2255, 106.799)
	if r8 != "888c107883fffff" || r6 != "868c1078fffffff" {
		t.Errorf("got r8=%s r6=%s, want 888c107883fffff / 868c1078fffffff", r8, r6)
	}
}
