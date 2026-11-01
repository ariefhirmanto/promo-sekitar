package promo

import (
	"errors"
	"regexp"
	"time"
)

var sourceIDPattern = regexp.MustCompile(`^src_[a-z0-9_]+$`)

type InputRun struct {
	SourceID string    // run only this source, due or not, whatever its status; empty = every due source
	Now      time.Time // the run's clock, for due checks and last_fetched_at
}

func (input InputRun) ValidateInput() error {
	if input.Now.IsZero() {
		return errors.New("now must be set")
	}
	if input.SourceID != "" && !sourceIDPattern.MatchString(input.SourceID) {
		return errors.New("source id must look like src_<a-z0-9_>")
	}
	return nil
}
