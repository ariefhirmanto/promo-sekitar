// Package provider is the registry of places vendor adapters.
//
// Each adapter lives in its own package (outlet/provider/<vendor>), implements
// outlet.PlaceProvider, and registers a factory under its name in init(), like database/sql
// drivers. job/app.go blank-imports the adapters and picks one by Places.Provider in config.yaml.
//
// Adding a vendor:
//  1. create outlet/provider/<vendor> with a client (fetch) and a transform (vendor → models.Place)
//  2. call provider.Register("<vendor>", factory) in its init()
//  3. blank-import it in job/app.go and set Places.Provider: "<vendor>"
package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"places-fetcher/config"
	"places-fetcher/outlet"
)

// Options are run-time settings shared by every adapter.
type Options struct {
	File    string // replay a saved vendor response instead of calling the vendor
	SaveRaw string // save the raw vendor response here
}

type Factory func(cfg config.MainConfig, opts Options) (outlet.PlaceProvider, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes an adapter available by name; it panics on a duplicate, like sql.Register.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[name]; dup {
		panic("provider: Register called twice for " + name)
	}
	factories[name] = f
}

// New builds the adapter registered under name.
func New(name string, cfg config.MainConfig, opts Options) (outlet.PlaceProvider, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown places provider %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return f(cfg, opts)
}

// Names lists the registered adapters.
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
