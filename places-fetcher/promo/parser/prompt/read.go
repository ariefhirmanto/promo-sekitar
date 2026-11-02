package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"places-fetcher/models"
)

// reply is Schema's shape; Read maps it to models.ParsedPromo.
type reply struct {
	Promos []struct {
		IsDeal         bool     `json:"is_deal"`
		BrandName      string   `json:"brand_name"`
		Title          string   `json:"title"`
		Deal           string   `json:"deal"`
		Channel        string   `json:"channel"`
		ValidFrom      string   `json:"valid_from"`
		ValidUntil     string   `json:"valid_until"`
		RecurrenceText string   `json:"recurrence_text"`
		OutletLimits   []limit  `json:"outlet_limits"`
		PaymentMethods []string `json:"payment_methods"`
		Category       string   `json:"category"`
		Confidence     float64  `json:"confidence"`
	} `json:"promos"`
}

type limit struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Read decodes a reply and checks it: the enums (for providers that don't enforce Schema), date
// formats and order, the confidence range, and that a deal has a title and a deal text. The error
// lists every problem, so Retry can send it back to the model as is.
func Read(text string) (resp []models.ParsedPromo, err error) {
	var r reply
	if err = json.Unmarshal([]byte(stripFence(text)), &r); err != nil {
		return nil, fmt.Errorf("reply is not valid JSON: %v", err)
	}

	var problems []string
	bad := func(i int, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("promos[%d].", i)+fmt.Sprintf(format, args...))
	}
	for i, p := range r.Promos {
		from, okFrom := date(p.ValidFrom)
		until, okUntil := date(p.ValidUntil)
		if !okFrom {
			bad(i, "valid_from: want YYYY-MM-DD or empty, got %q", p.ValidFrom)
		}
		if !okUntil {
			bad(i, "valid_until: want YYYY-MM-DD or empty, got %q", p.ValidUntil)
		}
		if okFrom && okUntil && !from.IsZero() && !until.IsZero() && until.Before(from) {
			bad(i, "valid_until %s is before valid_from %s", p.ValidUntil, p.ValidFrom)
		}
		if p.Confidence < 0 || p.Confidence > 1 {
			bad(i, "confidence: want 0 to 1, got %v", p.Confidence)
		}
		if !oneOf(p.Channel, Channels) {
			bad(i, "channel: want one of %s, got %q", strings.Join(Channels, ", "), p.Channel)
		}
		if !oneOf(p.Category, Categories) {
			bad(i, "category: want one of %s, got %q", strings.Join(Categories, ", "), p.Category)
		}
		if p.IsDeal && (strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Deal) == "") {
			bad(i, "a deal needs a title and a deal text")
		}

		pp := models.ParsedPromo{
			IsDeal: p.IsDeal, BrandName: strings.TrimSpace(p.BrandName), Title: p.Title, Deal: p.Deal,
			Channel: p.Channel, ValidFrom: p.ValidFrom, ValidUntil: p.ValidUntil, Recurrence: p.RecurrenceText,
			PaymentMethods: nonEmpty(p.PaymentMethods), Category: p.Category, Confidence: p.Confidence,
		}
		for _, l := range p.OutletLimits {
			if !oneOf(l.Kind, LimitKinds) {
				bad(i, "outlet_limits kind: want one of %s, got %q", strings.Join(LimitKinds, ", "), l.Kind)
			}
			if name := strings.TrimSpace(l.Name); name != "" {
				pp.OutletLimits = append(pp.OutletLimits, models.OutletLimit{Kind: l.Kind, Name: name})
			}
		}
		resp = append(resp, pp)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return resp, nil
}

// date accepts YYYY-MM-DD or empty (the zero time).
func date(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, true
	}
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func nonEmpty(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stripFence drops a ```json … ``` fence around the reply, which providers without enforced
// JSON output sometimes add.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}
