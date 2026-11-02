// Package claude is the promo parser on the Claude API (Haiku by default): one Messages call per
// raw item, with structured outputs, low effort and the shared instructions in the prompt cache.
package claude

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"places-fetcher/config"
	"places-fetcher/models"
	"places-fetcher/promo"
	"places-fetcher/promo/parser"
	"places-fetcher/promo/parser/prompt"

	"github.com/anthropics/anthropic-sdk-go"
	pkgerrors "github.com/pkg/errors"
)

const Name = "claude"

func init() {
	parser.Register(Name, func(cfg config.MainConfig) (promo.Parser, error) { return New(cfg.Claude) })
}

type claudeParser struct {
	cfg    config.ClaudeConfig
	client messenger
}

// New fails at startup without an API key, so a run never fetches and then can't parse.
func New(cfg config.ClaudeConfig) (*claudeParser, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is not set (or run with -parse=false)")
	}
	cfg = withDefaults(cfg)
	return &claudeParser{cfg: cfg, client: newSDKClient(cfg.APIKey, time.Duration(cfg.TimeoutSeconds)*time.Second)}, nil
}

func withDefaults(cfg config.ClaudeConfig) config.ClaudeConfig {
	if cfg.Model == "" {
		cfg.Model = string(anthropic.ModelClaudeHaiku5_5)
	}
	if cfg.Effort == "" {
		cfg.Effort = "low"
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	return cfg
}

// Parse runs the shared ask-check-retry loop (prompt.Run) over the Messages API; the retry
// continues the same conversation. Refusals, cut-off replies, a 400 for this request and a second
// bad reply are the item's failure (*models.ParseError); any other API error means parsing can't
// work this run.
func (p *claudeParser) Parse(ctx context.Context, req models.ParseRequest) (resp models.ParseResult, err error) {
	params := p.request(req)
	var last *anthropic.Message
	resp.Promos, err = prompt.Run(ctx, req.Item.ID, func(ctx context.Context, retry string) (string, error) {
		if retry != "" {
			params.Messages = append(params.Messages, last.ToParam(), anthropic.NewUserMessage(anthropic.NewTextBlock(retry)))
		}
		msg, err := p.client.New(ctx, params)
		if err != nil {
			var apiErr *anthropic.Error
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
				return "", &models.ParseError{Reason: "request refused: " + apiErr.Error()}
			}
			// every attempt for this item timed out while the run still has time: the item's
			// failure, tried again next run, so one slow reply doesn't stop the others
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				log.Printf("[Promo][Parse][Claude] Item %s timed out after %d attempts", req.Item.ID, retries+1)
				return "", &models.ParseError{Reason: fmt.Sprintf("timed out after %d attempts", retries+1)}
			}
			log.Println("[Promo][Parse][Claude] Problem calling the Messages API, err: ", err.Error())
			return "", pkgerrors.Wrap(err, "claude.Parse.Messages.New")
		}
		last = msg
		resp.CostUSD += p.cost(msg.Usage)
		resp.CacheReadTokens += msg.Usage.CacheReadInputTokens

		switch msg.StopReason {
		case anthropic.StopReasonRefusal:
			return "", &models.ParseError{Reason: "the model refused the item"}
		case anthropic.StopReasonMaxTokens:
			return "", &models.ParseError{Reason: fmt.Sprintf("reply cut off at %d tokens", p.cfg.MaxTokens)}
		}
		return replyText(msg), nil
	})
	return resp, err
}

// cost prices one call: cache writes at 1.25× and cache reads at 0.1× the input price.
func (p *claudeParser) cost(u anthropic.Usage) float64 {
	in := float64(u.InputTokens) + 1.25*float64(u.CacheCreationInputTokens) + 0.1*float64(u.CacheReadInputTokens)
	return (in*p.cfg.InputUSDPerMTok + float64(u.OutputTokens)*p.cfg.OutputUSDPerMTok) / 1e6
}

// replyText joins the text blocks; thinking blocks are skipped.
func replyText(msg *anthropic.Message) string {
	var b strings.Builder
	for _, c := range msg.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
