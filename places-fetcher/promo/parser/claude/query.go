package claude

import (
	"places-fetcher/models"
	"places-fetcher/promo/parser/prompt"

	"github.com/anthropics/anthropic-sdk-go"
)

// request lays out the shared blocks for the Messages API: blocks 1 and 2 as system text with
// the one cache marker after block 2, block 3 as the user message, the schema as structured output.
func (p *claudeParser) request(req models.ParseRequest) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:     anthropic.Model(p.cfg.Model),
		MaxTokens: int64(p.cfg.MaxTokens),
		System: []anthropic.TextBlockParam{
			{Text: prompt.Instructions},
			{Text: prompt.Context(req), CacheControl: anthropic.NewCacheControlEphemeralParam()},
		},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.Item(req)))},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(p.cfg.Effort),
			Format: anthropic.JSONOutputFormatParam{Schema: prompt.Schema},
		},
	}
}
