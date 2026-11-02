package prompt

import (
	"context"
	"log"

	"places-fetcher/models"
)

// Call sends one request for the item and returns the reply text. retry is empty on the first
// request; on the second it is the follow-up message (Retry), which the adapter adds to the same
// conversation after the model's first reply. An error is returned by Run as is, so the adapter
// decides whether it is the item's failure (*models.ParseError) or means the service can't work.
type Call func(ctx context.Context, retry string) (text string, err error)

// Run asks once and checks the reply with Read; on problems it asks once more with them. A
// second bad reply is the item's failure.
func Run(ctx context.Context, itemID string, call Call) (resp []models.ParsedPromo, err error) {
	text, err := call(ctx, "")
	if err != nil {
		return nil, err
	}
	resp, problem := Read(text)
	if problem == nil {
		return resp, nil
	}
	log.Printf("[Promo][Parse][Prompt] Item %s reply 1 invalid: %s", itemID, problem.Error())

	if text, err = call(ctx, Retry(problem)); err != nil {
		return nil, err
	}
	if resp, problem = Read(text); problem == nil {
		return resp, nil
	}
	log.Printf("[Promo][Parse][Prompt] Item %s reply 2 invalid: %s", itemID, problem.Error())
	return nil, &models.ParseError{Reason: "invalid reply after a retry: " + problem.Error()}
}
