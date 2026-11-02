package claude

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo/parser"
	"places-fetcher/promo/parser/prompt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const okReply = `{"promos":[{"is_deal":true,"brand_name":"Mie Contoh","title":"Beli 2 Rp35.000","deal":"2 mangkok Rp35.000",
"channel":"dine_in","valid_from":"2026-11-03","valid_until":"2026-11-09","recurrence_text":"setiap Jumat",
"outlet_limits":[{"kind":"mall","name":"Mal Contoh Raya"},{"kind":"city","name":" "}],"payment_methods":["QRIS",""],
"category":"food","confidence":0.95}]}`

// message builds an API reply the way the SDK decodes one.
func message(t *testing.T, stop, text string, usage string) *anthropic.Message {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-haiku-5-5", "stop_reason": stop,
		"content": []any{map[string]any{"type": "thinking", "thinking": "…", "signature": "sig"}, map[string]any{"type": "text", "text": text}},
		"usage":   json.RawMessage(usage),
	})
	var m anthropic.Message
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

const usage = `{"input_tokens":200,"output_tokens":300,"cache_creation_input_tokens":0,"cache_read_input_tokens":1000}`

type fakeMessenger struct {
	replies []*anthropic.Message
	got     []anthropic.MessageNewParams
}

func (f *fakeMessenger) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	f.got = append(f.got, params)
	m := f.replies[0]
	f.replies = f.replies[1:]
	return m, nil
}

func testParser(client messenger) *claudeParser {
	cfg := withDefaults(config.ClaudeConfig{APIKey: "test", InputUSDPerMTok: 0.10, OutputUSDPerMTok: 0.50})
	return &claudeParser{cfg: cfg, client: client}
}

var req = models.ParseRequest{
	Item:    models.RawItem{ID: "src_a:1001", URL: "https://example.com/1001", PostedAt: "2026-11-01T10:00:00+07:00", Text: "𝗣𝗥𝗢𝗠𝗢 Mie Contoh"},
	Today:   "2026-12-15",
	Brand:   "Mie Contoh",
	Outlets: []string{"Mal Contoh Raya", "Mie Contoh Bekasi"},
}

func TestParseReadsReply(t *testing.T) {
	m := &fakeMessenger{replies: []*anthropic.Message{message(t, "end_turn", okReply, usage)}}
	res, err := testParser(m).Parse(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Promos) != 1 {
		t.Fatalf("want 1 promo: %+v", res)
	}
	p := res.Promos[0]
	want := models.ParsedPromo{IsDeal: true, BrandName: "Mie Contoh", Title: "Beli 2 Rp35.000", Deal: "2 mangkok Rp35.000",
		Channel: "dine_in", ValidFrom: "2026-11-03", ValidUntil: "2026-11-09", Recurrence: "setiap Jumat",
		OutletLimits:   []models.OutletLimit{{Kind: "mall", Name: "Mal Contoh Raya"}},
		PaymentMethods: []string{"QRIS"}, Category: "food", Confidence: 0.95}
	gotJSON, _ := json.Marshal(p)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("transform:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	// 200 input + 1000 cached at 0.1× = 300 input-priced tokens; 300 output
	if wantCost := (300*0.10 + 300*0.50) / 1e6; math.Abs(res.CostUSD-wantCost) > 1e-12 || res.CacheReadTokens != 1000 {
		t.Fatalf("cost %v (want %v), cache read %d", res.CostUSD, wantCost, res.CacheReadTokens)
	}
}

func TestRequestLayout(t *testing.T) {
	params := testParser(nil).request(req)
	if params.Model != "claude-haiku-5-5" || params.MaxTokens != 4096 || params.OutputConfig.Effort != "low" {
		t.Fatalf("model settings: %v %v %v", params.Model, params.MaxTokens, params.OutputConfig.Effort)
	}
	if params.OutputConfig.Format.Schema["additionalProperties"] != false {
		t.Fatal("the reply schema goes in output_config.format")
	}
	if len(params.System) != 2 || params.System[0].Text != prompt.Instructions {
		t.Fatal("block 1 is the fixed instructions")
	}
	b2 := params.System[1]
	if !strings.Contains(b2.Text, "belongs to Mie Contoh") || !strings.Contains(b2.Text, "- Mal Contoh Raya\n- Mie Contoh Bekasi") {
		t.Fatalf("block 2 is the brand and its outlets: %q", b2.Text)
	}
	body, _ := json.Marshal(params)
	if strings.Count(string(body), `"cache_control"`) != 1 || !strings.Contains(string(body), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("one cache marker, on block 2: %s", body)
	}
	var user struct {
		Messages []struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(body, &user)
	text := user.Messages[0].Content[0].Text
	if !strings.Contains(text, "Today: 2026-12-15") || !strings.Contains(text, "PROMO Mie Contoh") {
		t.Fatalf("block 3 has the date and the NFKC text: %q", text)
	}
	if strings.Contains(prompt.Instructions+b2.Text, "2026-12-15") {
		t.Fatal("the cached blocks must not hold the date")
	}

	agg := testParser(nil).request(models.ParseRequest{Item: req.Item, Today: req.Today, KnownBrands: []string{"Kopi Contoh", "Mie Contoh"}})
	if !strings.Contains(agg.System[1].Text, "aggregator") || !strings.Contains(agg.System[1].Text, "- Kopi Contoh\n- Mie Contoh") {
		t.Fatalf("an aggregator's block 2 lists tracked brands: %q", agg.System[1].Text)
	}
}

func TestParseRetriesOnceWithTheProblems(t *testing.T) {
	bad := strings.Replace(okReply, `"valid_until":"2026-11-09"`, `"valid_until":"9 Nov"`, 1)
	m := &fakeMessenger{replies: []*anthropic.Message{message(t, "end_turn", bad, usage), message(t, "end_turn", okReply, usage)}}
	res, err := testParser(m).Parse(context.Background(), req)
	if err != nil || len(res.Promos) != 1 {
		t.Fatalf("second reply is fine: %+v %v", res, err)
	}
	if len(m.got) != 2 || len(m.got[1].Messages) != 3 {
		t.Fatalf("the retry continues the conversation: %d calls", len(m.got))
	}
	body, _ := json.Marshal(m.got[1].Messages[2])
	if !strings.Contains(string(body), `valid_until: want YYYY-MM-DD or empty, got \"9 Nov\"`) {
		t.Fatalf("the retry names the problem: %s", body)
	}
	if res.CostUSD <= 0 || res.CacheReadTokens != 2000 {
		t.Fatalf("both calls are counted: %+v", res)
	}

	m = &fakeMessenger{replies: []*anthropic.Message{message(t, "end_turn", bad, usage), message(t, "end_turn", bad, usage)}}
	res, err = testParser(m).Parse(context.Background(), req)
	var perr *models.ParseError
	if !errors.As(err, &perr) || !strings.Contains(perr.Reason, "after a retry") || res.CostUSD <= 0 || len(res.Promos) != 0 {
		t.Fatalf("a second bad reply fails the item, cost still counted: %+v %v", res, err)
	}
}

func TestParseStopReasonsFailTheItem(t *testing.T) {
	for _, stop := range []string{"refusal", "max_tokens"} {
		m := &fakeMessenger{replies: []*anthropic.Message{message(t, stop, `{"promos":[`, usage)}}
		_, err := testParser(m).Parse(context.Background(), req)
		var perr *models.ParseError
		if !errors.As(err, &perr) {
			t.Fatalf("%s must be an item failure: %v", stop, err)
		}
	}
}

// API errors: a 400 belongs to the item, anything else stops the parse stage.
func TestParseAPIErrors(t *testing.T) {
	for code, itemFault := range map[int]bool{400: true, 401: false, 529: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			w.Write([]byte(`{"type":"error","error":{"type":"x","message":"from test"}}`))
		}))
		p := testParser(newSDKClient("test", 5*time.Second, option.WithBaseURL(srv.URL), option.WithMaxRetries(0)))
		_, err := p.Parse(context.Background(), req)
		srv.Close()
		var perr *models.ParseError
		if err == nil || errors.As(err, &perr) != itemFault {
			t.Errorf("HTTP %d: item fault %v, got %v", code, itemFault, err)
		}
	}
}

func TestRegistry(t *testing.T) {
	if _, err := parser.New(Name, config.MainConfig{}); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("no key must fail at startup: %v", err)
	}
	p, err := parser.New(Name, config.MainConfig{Claude: config.ClaudeConfig{APIKey: "test"}})
	if err != nil || p == nil {
		t.Fatalf("registered under %q: %v", Name, err)
	}
	if _, err := parser.New("gpt", config.MainConfig{}); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("an unknown name lists the registered ones: %v", err)
	}
}

// hangingServer stalls the first `hang` requests past the client's timeout, then answers okReply.
func hangingServer(t *testing.T, hang int) (*httptest.Server, *int) {
	calls := 0
	body, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-haiku-5-5", "stop_reason": "end_turn",
		"content": []map[string]any{{"type": "text", "text": okReply}},
		"usage":   map[string]any{"input_tokens": 10, "output_tokens": 10},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= hang {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestParseTimeouts(t *testing.T) {
	// one attempt hangs: the SDK tries again and the item parses
	srv, calls := hangingServer(t, 1)
	p := testParser(newSDKClient("test", 200*time.Millisecond, option.WithBaseURL(srv.URL)))
	if resp, err := p.Parse(context.Background(), req); err != nil || len(resp.Promos) != 1 || *calls != 2 {
		t.Fatalf("hung attempt retried: %d calls, %v, %+v", *calls, err, resp)
	}

	// every attempt hangs: this item fails, the run goes on
	srv, calls = hangingServer(t, 99)
	p = testParser(newSDKClient("test", 200*time.Millisecond, option.WithBaseURL(srv.URL)))
	_, err := p.Parse(context.Background(), req)
	var perr *models.ParseError
	if !errors.As(err, &perr) || *calls != retries+1 {
		t.Fatalf("all attempts timed out: want the item's failure after %d calls, got %d, %v", retries+1, *calls, err)
	}

	// the run itself is out of time: not the item's fault, parsing stops
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = p.Parse(ctx, req)
	if err == nil || errors.As(err, &perr) {
		t.Fatalf("run deadline: want a plain error, got %v", err)
	}
}
