package picker

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/idealland-apps/Blink/internal/model"
)

func TestPickExcludesRecentEntriesAndRotatesFeeds(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	entries := []model.Entry{
		{ID: 1, FeedID: 10, Title: "recently shown", ReadingTime: 2, PublishedAt: now.Add(-time.Hour)},
		{ID: 2, FeedID: 10, Title: "same feed", ReadingTime: 2, PublishedAt: now.Add(-time.Hour)},
		{ID: 3, FeedID: 20, Title: "rotated feed", ReadingTime: 2, PublishedAt: now.Add(-time.Hour)},
	}

	got, err := Pick(entries, Options{Now: now, RecentEntryIDs: map[int64]bool{1: true}, RecentFeedIDs: map[int64]bool{10: true}, RNG: rand.New(rand.NewPCG(1, 2))})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 3 {
		t.Fatalf("picked entry %d, want 3", got.ID)
	}
}

func TestPickPrefersRequestedReadingTime(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	entries := []model.Entry{
		{ID: 1, FeedID: 10, Title: "long", ReadingTime: 20, PublishedAt: now.Add(-time.Hour)},
		{ID: 2, FeedID: 20, Title: "short", ReadingTime: 3, PublishedAt: now.Add(-time.Hour)},
	}

	got, err := Pick(entries, Options{Now: now, Minutes: 3, RNG: rand.New(rand.NewPCG(1, 2))})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 2 {
		t.Fatalf("picked entry %d, want short entry 2", got.ID)
	}
}

func TestPickRejectsEntriesOutsideFreshnessWindow(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	_, err := Pick([]model.Entry{{ID: 1, Title: "old", PublishedAt: now.AddDate(0, 0, -31)}}, Options{Now: now, Freshness: 7 * 24 * time.Hour, RNG: rand.New(rand.NewPCG(1, 2))})
	if err == nil {
		t.Fatal("Pick returned nil error for an empty eligible set")
	}
}
