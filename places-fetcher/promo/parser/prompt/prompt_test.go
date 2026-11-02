package prompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"places-fetcher/models"
)

func TestReadChecks(t *testing.T) {
	cases := map[string]string{
		`{"promos":[{"is_deal":true,"title":"","deal":"x","channel":"all","valid_from":"","valid_until":"","category":"food","confidence":0.9}]}`:                      "needs a title",
		`{"promos":[{"is_deal":true,"title":"a","deal":"x","channel":"all","valid_from":"2026-11-09","valid_until":"2026-11-01","category":"food","confidence":0.9}]}`: "before valid_from",
		`{"promos":[{"is_deal":true,"title":"a","deal":"x","channel":"all","valid_from":"","valid_until":"","category":"food","confidence":1.5}]}`:                     "confidence",
		`{"promos":[{"is_deal":true,"title":"a","deal":"x","channel":"pickup","valid_from":"","valid_until":"","category":"food","confidence":0.5}]}`:                  "channel",
		`not json`: "not valid JSON",
	}
	for in, want := range cases {
		if _, err := Read(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want error containing %q, got %v", in, want, err)
		}
	}
	got, err := Read(`{"promos":[]}`)
	if err != nil || len(got) != 0 {
		t.Fatalf("no promos is a fine reply: %v %v", got, err)
	}
}

func TestReadEnumsAndFence(t *testing.T) {
	// a provider without schema enforcement may wrap the JSON and invent values
	fenced := "```json\n" + `{"promos":[{"is_deal":true,"title":"a","deal":"x","channel":"all","valid_from":"","valid_until":"","category":"snack","confidence":0.5,"outlet_limits":[{"kind":"street","name":"Jl. Contoh"}]}]}` + "\n```"
	_, err := Read(fenced)
	if err == nil || !strings.Contains(err.Error(), "category") || !strings.Contains(err.Error(), "outlet_limits kind") {
		t.Fatalf("the fence is stripped and the enums are checked: %v", err)
	}
}

const good = `{"promos":[{"is_deal":true,"brand_name":"Mie Contoh","title":"a","deal":"Diskon 10%","channel":"all","valid_from":"","valid_until":"","recurrence_text":"","outlet_limits":[],"payment_methods":[],"category":"food","confidence":0.9}]}`

func TestRunRetriesOnceThenFails(t *testing.T) {
	var retries []string
	replies := []string{`{"promos":[{"is_deal":true,"title":"","deal":"x","channel":"all","category":"food","confidence":0.9}]}`, good}
	call := func(ctx context.Context, retry string) (string, error) {
		retries = append(retries, retry)
		r := replies[0]
		replies = replies[1:]
		return r, nil
	}
	got, err := Run(context.Background(), "src_a:1", call)
	if err != nil || len(got) != 1 || retries[0] != "" || !strings.Contains(retries[1], "needs a title") {
		t.Fatalf("the second call carries the problems: %v %v %q", got, err, retries)
	}

	bad := func(ctx context.Context, retry string) (string, error) { return "not json", nil }
	_, err = Run(context.Background(), "src_a:1", bad)
	var perr *models.ParseError
	if !errors.As(err, &perr) || !strings.Contains(perr.Reason, "after a retry") {
		t.Fatalf("a second bad reply fails the item: %v", err)
	}

	down := errors.New("401")
	calls := 0
	_, err = Run(context.Background(), "src_a:1", func(ctx context.Context, retry string) (string, error) { calls++; return "", down })
	if err != down || calls != 1 {
		t.Fatalf("a call error is returned as is, without a retry: %v after %d calls", err, calls)
	}
}

func TestBlocks(t *testing.T) {
	req := models.ParseRequest{Item: models.RawItem{URL: "https://example.com/1", Text: "𝗣𝗥𝗢𝗠𝗢"}, Today: "2026-12-15",
		Brand: "Mie Contoh", Outlets: []string{"A", "B"}}
	if c := Context(req); !strings.Contains(c, "belongs to Mie Contoh") || !strings.Contains(c, "- A\n- B\n") {
		t.Fatalf("brand block: %q", c)
	}
	if i := Item(req); !strings.Contains(i, "Today: 2026-12-15") || !strings.Contains(i, "PROMO") ||
		!strings.Contains(i, "(not stated; this is the date it was read)") {
		t.Fatalf("item block: %q", i)
	}
	if strings.Contains(Instructions, "2026-12-15") || strings.Contains(Context(req), "2026-12-15") {
		t.Fatal("blocks 1 and 2 never hold the run's date")
	}
}
