package source

import (
	"context"
	"errors"
	"testing"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo"
)

type stub struct{ name string }

func (s stub) Type() string { return s.name }
func (s stub) FetchItems(ctx context.Context, req models.FetchRequest) (models.FetchResult, error) {
	return models.FetchResult{}, nil
}

// reset swaps the registry for the test and restores it afterwards.
func reset(t *testing.T) {
	t.Helper()
	saved := factories
	factories = map[string]Factory{}
	t.Cleanup(func() { factories = saved })
}

func TestSetBuildsEveryRegisteredType(t *testing.T) {
	reset(t)
	Register("web_page", func(config.MainConfig) (promo.SourceAdapter, error) { return stub{"web_page"}, nil })
	Register("telegram", func(config.MainConfig) (promo.SourceAdapter, error) { return stub{"telegram"}, nil })

	s, err := NewSet(config.MainConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := s.ForType("web_page"); !ok || a.Type() != "web_page" {
		t.Fatal("web_page not found")
	}
	if _, ok := s.ForType("instagram"); ok {
		t.Fatal("an unregistered type must not be found")
	}
	if got := Names(); len(got) != 2 || got[0] != "telegram" {
		t.Fatalf("Names = %v", got)
	}
}

func TestNewSetFailsOnFactoryError(t *testing.T) {
	reset(t)
	Register("web_page", func(config.MainConfig) (promo.SourceAdapter, error) { return nil, errors.New("no user agent") })
	if _, err := NewSet(config.MainConfig{}); err == nil {
		t.Fatal("a factory error must stop the run at startup")
	}
}

func TestRegisterTwicePanics(t *testing.T) {
	reset(t)
	f := func(config.MainConfig) (promo.SourceAdapter, error) { return stub{"x"}, nil }
	Register("x", f)
	defer func() {
		if recover() == nil {
			t.Fatal("want a panic on a duplicate type")
		}
	}()
	Register("x", f)
}
