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

// sdkClient is the official SDK. It retries 429 and 5xx answers itself (2 retries, with backoff).
type sdkClient struct {
	client  anthropic.Client
	timeout time.Duration
}

func newSDKClient(apiKey string, timeout time.Duration, opts ...option.RequestOption) *sdkClient {
	opts = append([]option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(2)}, opts...)
	return &sdkClient{client: anthropic.NewClient(opts...), timeout: timeout}
}

func (c *sdkClient) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.client.Messages.New(ctx, params)
}
