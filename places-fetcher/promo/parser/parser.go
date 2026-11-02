// Package parser is the registry of promo parsers (raw item → promos).
//
// Each parser lives in its own package (promo/parser/<name>), implements promo.Parser, and
// registers a factory in init(), like database/sql drivers. Parser.Name in config.yaml picks one
// (-parser overrides it for a run), so switching between registered parsers is a config change.
//
// What every parser shares is in promo/parser/prompt: the instructions, the reply schema, the
// three request blocks, the checks on a reply (Read) and the ask-check-retry loop (Run). A parser
// only sends the blocks to its service, returns the reply text, and prices the call.
//
// Adding a parser:
//  1. create promo/parser/<name>: New(cfg) that fails without its key, and Parse that calls
//     prompt.Run with a Call sending prompt.Instructions, prompt.Context(req) and prompt.Item(req)
//     (plus prompt.Schema if the service enforces one). Return *models.ParseError for the item's
//     own failures (refusal, cut-off reply, 400) and a plain error for the rest; fill CostUSD
//  2. a <Name>Config section in config/config.go and config.yaml; its key from env, and the
//     secret in .github/workflows/promos_daily.yml
//  3. call parser.Register("<name>", factory) in its init(), and blank-import it in
//     job/promos.go and in golden_test.go
//  4. compare it with the current parser: go test -tags golden with PARSER=<name>, and a
//     -dry-run -parser <name>; then set Parser.Name
//
// A service with an OpenAI-compatible API (most hosted models and routers) is best added once
// as a generic adapter with BaseURL, Model and prices in its config section; after that, moving
// between such services is a config change.
package parser

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"places-fetcher/config"
	"places-fetcher/promo"
)

type Factory func(cfg config.MainConfig) (promo.Parser, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a parser available by name; it panics on a duplicate, like sql.Register.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[name]; dup {
		panic("parser: Register called twice for " + name)
	}
	factories[name] = f
}

// New builds the named parser; a factory error (e.g. no API key) stops the run at startup.
func New(name string, cfg config.MainConfig) (promo.Parser, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown parser %q (registered: %s)", name, strings.Join(Names(), ", "))
	}
	p, err := f(cfg)
	if err != nil {
		return nil, fmt.Errorf("parser %s: %w", name, err)
	}
	return p, nil
}

// Names lists the registered parsers.
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
