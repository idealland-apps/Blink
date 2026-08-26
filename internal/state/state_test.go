package state

import (
	"path/filepath"
	"testing"
)

func TestRecordLimitsRecentHistoryAndTracksFeed(t *testing.T) {
	state := State{}
	for i := int64(1); i <= 55; i++ {
		state.Record(i, i+100)
	}
	if len(state.RecentEntryIDs) != maxRecent {
		t.Fatalf("history len = %d, want %d", len(state.RecentEntryIDs), maxRecent)
	}
	if state.RecentEntryIDs[0] != 55 || state.RecentFeedIDs[0] != 155 {
		t.Fatalf("most recent state = %#v", state)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := State{}
	want.Record(5, 10)
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RecentEntryIDs) != 1 || got.RecentEntryIDs[0] != 5 {
		t.Fatalf("Load() = %#v", got)
	}
}
