package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const maxRecent = 50

type State struct {
	RecentEntryIDs []int64 `json:"recent_entry_ids"`
	RecentFeedIDs  []int64 `json:"recent_feed_ids"`
}

func (s *State) Record(entryID, feedID int64) {
	s.RecentEntryIDs = prepend(entryID, s.RecentEntryIDs)
	s.RecentFeedIDs = prepend(feedID, s.RecentFeedIDs)
}

func (s State) EntrySet() map[int64]bool { return asSet(s.RecentEntryIDs) }
func (s State) FeedSet() map[int64]bool  { return asSet(s.RecentFeedIDs) }

func prepend(id int64, ids []int64) []int64 {
	out := []int64{id}
	for _, existing := range ids {
		if existing != id {
			out = append(out, existing)
		}
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	return out
}

func asSet(ids []int64) map[int64]bool {
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func Save(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-state-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
