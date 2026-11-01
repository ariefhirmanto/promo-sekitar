package webpage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"places-fetcher/models"
)

// bcaRule reads a promo.bca.co.id category page (/id/all/c/<category>). The page is a Next.js app:
// its cards come as JSON inside self.__next_f.push([1,"…"]) script chunks, with the brand, title
// and exact publish/start/end dates. Each card's own page adds the terms (a "description" field
// holding HTML). Only page 1 (the newest cards) is read.
type bcaRule struct{}

// a card as it appears in the listing payload
type bcaCard struct {
	IDPromo     string `json:"idPromo"`
	BrandPromo  string `json:"brandPromo"`
	TitlePromo  string `json:"titlePromo"`
	LinkPage    string `json:"linkPage"` // e.g. all/2026/10/30/23/hokben, under /<locale>/
	PublishDate string `json:"publishDate"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
}

// the promo body on a card's own page
type bcaDetail struct {
	Description string `json:"description"`
}

var nextChunk = regexp.MustCompile(`self\.__next_f\.push\(\[1,"((?:[^"\\]|\\.)*)"\]\)`)

const bcaTime = "2006-01-02 15:04:05.000"

func (bcaRule) fetch(ctx context.Context, get fetcher, req models.FetchRequest, target *url.URL, maxNew int) (resp models.FetchResult, err error) {
	locale := strings.SplitN(strings.TrimPrefix(target.Path, "/"), "/", 2)[0]
	if locale == "" {
		locale = "id"
	}

	page, err := get.Get(ctx, target.String())
	if err != nil {
		return resp, err
	}
	payload, err := nextPayload(page)
	if err != nil {
		return resp, fmt.Errorf("bca %s: %w", target, err)
	}
	var cards []bcaCard
	if err := decodeEach(payload, `{"versionPage":`, func(dec *json.Decoder) error {
		var c bcaCard
		if err := dec.Decode(&c); err != nil {
			return err
		}
		if c.IDPromo != "" && c.LinkPage != "" {
			cards = append(cards, c)
		}
		return nil
	}); err != nil {
		return resp, fmt.Errorf("bca %s: reading cards: %w", target, err)
	}
	if len(cards) == 0 {
		return resp, fmt.Errorf("bca %s: no promo cards, the page layout changed", target)
	}

	var list []detail
	for _, c := range cards {
		c := c
		list = append(list, detail{
			ref: c.IDPromo,
			url: absolute(target, "/"+locale+"/"+strings.TrimPrefix(c.LinkPage, "/")),
			item: func(page []byte) (models.RawItem, error) {
				return bcaItem(c, page)
			},
		})
	}
	resp.Items, err = openDetails(ctx, get, req.Source, req.KnownRefs, dedupe(list), maxNew)
	return resp, err
}

// bcaItem joins the card's facts with the terms from its page.
func bcaItem(c bcaCard, page []byte) (it models.RawItem, err error) {
	payload, err := nextPayload(page)
	if err != nil {
		return it, err
	}
	// The page has several "description" fields (category blurbs too); the promo's is the first
	// that holds markup, either inline or as a "$<id>" reference to a text chunk.
	var terms string
	err = decodeEach(payload, `{"description":`, func(dec *json.Decoder) error {
		var d bcaDetail
		if err := dec.Decode(&d); err != nil {
			return err
		}
		if desc := resolveChunk(payload, d.Description); terms == "" && strings.Contains(desc, "<") {
			terms = htmlText(desc)
		}
		return nil
	})
	if err != nil {
		return it, err
	}
	if terms == "" {
		return it, fmt.Errorf("no promo description")
	}

	lines := []string{"Brand: " + c.BrandPromo, "Promo: " + c.TitlePromo}
	if from, until := bcaDate(c.StartDate), bcaDate(c.EndDate); from != "" || until != "" {
		lines = append(lines, fmt.Sprintf("Periode: %s sampai %s", from, until))
	}
	it.Text = strings.Join(append(lines, terms), "\n")
	if t, err := time.ParseInLocation(bcaTime, c.PublishDate, models.WIB); err == nil {
		it.PostedAt = t.Format(models.TimeLayout)
	}
	return it, nil
}

// nextPayload joins a Next.js page's streamed chunks into one string.
func nextPayload(page []byte) (string, error) {
	var b strings.Builder
	for _, m := range nextChunk.FindAllSubmatch(page, -1) {
		var s string
		if err := json.Unmarshal(append(append([]byte{'"'}, m[1]...), '"'), &s); err != nil {
			return "", fmt.Errorf("bad script chunk: %w", err)
		}
		b.WriteString(s)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no self.__next_f data")
	}
	return b.String(), nil
}

var chunkRef = regexp.MustCompile(`^\$([0-9a-f]+)$`)

// resolveChunk turns a "$2a" reference into the text chunk it names: in the payload that chunk is
// written as "2a:T<length in hex>,<text>". Anything else is returned as it is.
func resolveChunk(payload, value string) string {
	m := chunkRef.FindStringSubmatch(value)
	if m == nil {
		return value
	}
	head := m[1] + ":T"
	i := strings.Index(payload, "\n"+head)
	if i >= 0 {
		i++
	} else if strings.HasPrefix(payload, head) {
		i = 0
	} else {
		return ""
	}
	rest := payload[i+len(head):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return ""
	}
	n, err := strconv.ParseInt(rest[:comma], 16, 64)
	if err != nil || n < 0 {
		return ""
	}
	body := rest[comma+1:]
	if int(n) < len(body) {
		body = body[:n]
	}
	return body
}

// decodeEach decodes one JSON value at every place the payload contains prefix.
func decodeEach(payload, prefix string, decode func(*json.Decoder) error) error {
	for i := strings.Index(payload, prefix); i >= 0; {
		if err := decode(json.NewDecoder(strings.NewReader(payload[i:]))); err != nil {
			return err
		}
		next := strings.Index(payload[i+len(prefix):], prefix)
		if next < 0 {
			break
		}
		i += len(prefix) + next
	}
	return nil
}

// bcaDate turns "2026-11-05 00:01:00.000" into "2026-11-05"; empty stays empty.
func bcaDate(s string) string {
	if t, err := time.Parse(bcaTime, s); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}
