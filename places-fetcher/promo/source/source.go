// Package source is the registry of promo source types.
//
// Each type lives in its own package (promo/source/<type>), implements promo.SourceAdapter, and
// registers a factory under its type name in init(), like database/sql drivers. Unlike places
// vendors, where config picks one, a run needs every type at once: NewSet builds all registered
// adapters and hands the usecase the one matching each sources row's type.
//
// Adding a type:
//  1. create promo/source/<type> with a client (fetch) and a transform (page → models.RawItem)
//  2. call source.Register("<type>", factory) in its init()
//  3. blank-import it in job/promos.go
package source

import (
	"fmt"
	"sort"
	"sync"

	"places-fetcher/config"
	"places-fetcher/promo"
)

type Factory func(cfg config.MainConfig) (promo.SourceAdapter, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a source type available by name; it panics on a duplicate, like sql.Register.
func Register(sourceType string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[sourceType]; dup {
		panic("source: Register called twice for " + sourceType)
	}
	factories[sourceType] = f
}

// Names lists the registered source types.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Set holds one built adapter per registered type.
type Set struct {
	adapters map[string]promo.SourceAdapter
}

// NewSet builds every registered adapter; a factory error (e.g. missing config) stops the run at startup.
func NewSet(cfg config.MainConfig) (*Set, error) {
	s := &Set{adapters: map[string]promo.SourceAdapter{}}
	for _, name := range Names() {
		mu.RLock()
		f := factories[name]
		mu.RUnlock()
		a, err := f(cfg)
		if err != nil {
			return nil, fmt.Errorf("source type %s: %w", name, err)
		}
		s.adapters[name] = a
	}
	return s, nil
}

func (s *Set) ForType(sourceType string) (promo.SourceAdapter, bool) {
	a, ok := s.adapters[sourceType]
	return a, ok
}
