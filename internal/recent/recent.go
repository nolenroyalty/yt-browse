package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxEntries = 100

type Entry struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Handle   string    `json:"handle"`
	LastUsed time.Time `json:"last_used"`
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads recent entries from disk, sorted by LastUsed descending.
// Returns an empty slice for file-not-found, nil for corrupt/unreadable JSON.
// Callers that write (Add, Remove) check for nil to avoid overwriting a
// corrupt file with partial data.
func (s *Store) Load() []Entry {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}
		}
		return nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastUsed.After(entries[j].LastUsed)
	})
	return entries
}

// Add upserts a channel entry, bumps LastUsed, trims to maxEntries, and writes.
func (s *Store) Add(id, title, handle string) {
	entries := s.Load()
	if entries == nil {
		return // corrupt file — don't overwrite with partial data
	}

	// Upsert
	found := false
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Title = title
			entries[i].Handle = handle
			entries[i].LastUsed = time.Now()
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, Entry{
			ID:       id,
			Title:    title,
			Handle:   handle,
			LastUsed: time.Now(),
		})
	}

	// Sort newest first, trim
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastUsed.After(entries[j].LastUsed)
	})
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}

	// Write (silently ignore errors)
	s.write(entries)
}

// Remove deletes the entry with the given ID and writes the updated list.
func (s *Store) Remove(id string) {
	entries := s.Load()
	if entries == nil {
		return // corrupt file — don't overwrite with partial data
	}
	for i := range entries {
		if entries[i].ID == id {
			entries = append(entries[:i], entries[i+1:]...)
			s.write(entries)
			return
		}
	}
}

func (s *Store) write(entries []Entry) {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = os.WriteFile(s.path, data, 0o644)
}
