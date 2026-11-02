package usecase

import (
	"fmt"
	"time"

	"places-fetcher/models"
)

const maxDueSlack = 2 * time.Hour

// IsDue reports whether an active source's fetch_every_hours has passed since last_fetched_at.
// A run may start a little earlier than the last one (GitHub cron drifts), so the interval is
// shortened by 10% of itself, at most 2 hours: a daily source fetched at 03:01 is due again from
// 01:01 the next day, a 6-hourly one after 5h24m.
func IsDue(s models.Source, now time.Time) bool {
	if s.Status != "active" {
		return false
	}
	if s.LastFetchedAt == "" {
		return true
	}
	last, err := time.Parse(models.TimeLayout, s.LastFetchedAt)
	if err != nil {
		return true // unreadable timestamp: fetch rather than stall
	}
	every := time.Duration(s.FetchEveryHours) * time.Hour
	if every <= 0 {
		every = 24 * time.Hour
	}
	slack := every / 10
	if slack > maxDueSlack {
		slack = maxDueSlack
	}
	return now.Sub(last) >= every-slack
}

// DueSources picks the sources this run fetches: every due one, or only sourceID (due or not).
func DueSources(sources []models.Source, sourceID string, now time.Time) ([]models.Source, error) {
	var due []models.Source
	for _, s := range sources {
		if sourceID != "" {
			if s.ID == sourceID {
				return []models.Source{s}, nil
			}
			continue
		}
		if IsDue(s, now) {
			due = append(due, s)
		}
	}
	if sourceID != "" {
		return nil, fmt.Errorf("no source %s in the sources tab", sourceID)
	}
	return due, nil
}
