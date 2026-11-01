package models

import "strings"

// base model, one row of the brands tab
type Brand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Aliases string `json:"aliases"` // comma-separated, as in the sheet
}

// Names returns the brand name followed by its aliases.
func (b Brand) Names() []string {
	names := []string{b.Name}
	for _, a := range strings.Split(b.Aliases, ",") {
		if a = strings.TrimSpace(a); a != "" {
			names = append(names, a)
		}
	}
	return names
}
