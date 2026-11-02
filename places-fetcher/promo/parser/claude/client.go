package claude

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// messenger is the one Messages API call the parser makes; tests swap in a fake.
type messenger interface {
	New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

// retries is how often the SDK tries a call again: a 429 or 5xx answer, or an attempt that hit
// the timeout.
const retries = 2

// sdkClient is the official SDK. timeout applies to each attempt, so a reply that hangs is
// retried (with backoff) instead of using up the whole call.
type sdkClient struct {
	client  anthropic.Client
	timeout time.Duration
}

func newSDKClient(apiKey string, timeout time.Duration, opts ...option.RequestOption) *sdkClient {
	opts = append([]option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(retries),
		option.WithRequestTimeout(timeout)}, opts...)
	return &sdkClient{client: anthropic.NewClient(opts...), timeout: timeout}
}

func (c *sdkClient) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	// every attempt, plus room for the backoff between them
	ctx, cancel := context.WithTimeout(ctx, (retries+1)*c.timeout+30*time.Second)
	defer cancel()
	return c.client.Messages.New(ctx, params)
}
