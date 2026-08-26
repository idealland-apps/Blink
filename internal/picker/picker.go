package picker

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/idealland-apps/Blink/internal/model"
)

type Options struct {
	Now            time.Time
	Freshness      time.Duration
	Minutes        int
	RecentEntryIDs map[int64]bool
	RecentFeedIDs  map[int64]bool
	RNG            *rand.Rand
}

func Pick(entries []model.Entry, options Options) (model.Entry, error) {
	eligible := make([]model.Entry, 0, len(entries))
	for _, entry := range entries {
		if options.RecentEntryIDs[entry.ID] {
			continue
		}
		if options.Freshness > 0 && entry.PublishedAt.Before(options.Now.Add(-options.Freshness)) {
			continue
		}
		eligible = append(eligible, entry)
	}
	if len(eligible) == 0 {
		return model.Entry{}, errors.New("no eligible entries found")
	}
	rotated := eligible[:0]
	for _, entry := range eligible {
		if !options.RecentFeedIDs[entry.FeedID] {
			rotated = append(rotated, entry)
		}
	}
	if len(rotated) > 0 {
		eligible = rotated
	}
	if options.Minutes > 0 {
		withinTime := make([]model.Entry, 0, len(eligible))
		for _, entry := range eligible {
			if entry.ReadingTime > 0 && entry.ReadingTime <= options.Minutes {
				withinTime = append(withinTime, entry)
			}
		}
		if len(withinTime) > 0 {
			eligible = withinTime
		}
	}
	rng := options.RNG
	if rng == nil {
		rng = rand.New(rand.NewPCG(uint64(options.Now.UnixNano()), 1))
	}
	return eligible[rng.IntN(len(eligible))], nil
}
