// Package history keeps every version of the configuration file so that
// changes can be inspected and rolled back.
package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Entry describes one stored version.
type Entry struct {
	ID       int64     `json:"id"`
	Time     time.Time `json:"time"`
	User     string    `json:"user,omitempty"`
	Message  string    `json:"message,omitempty"`
	Kind     string    `json:"kind"` // initial, change, external, rollback, raw
	Ops      []string  `json:"ops,omitempty"`
	Hash     string    `json:"hash"`
	Size     int       `json:"size"`
	Reloaded bool      `json:"reloaded"`
	Added    int       `json:"added"`
	Removed  int       `json:"removed"`
}

// Store saves versions in a directory.
type Store struct {
	Dir  string
	Keep int // number of versions to keep, 0 = unlimited
	mu   sync.Mutex
}

// Hash returns the content hash used for versions.
func Hash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

// Open creates the directory if needed.
func Open(dir string, keep int) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, Keep: keep}, nil
}

func (s *Store) ids() ([]int64, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(name, ".json"), 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (s *Store) path(id int64, ext string) string {
	return filepath.Join(s.Dir, fmt.Sprintf("%08d%s", id, ext))
}

// Save stores a new version.
func (s *Store) Save(content string, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.ids()
	if err != nil {
		return e, err
	}
	e.ID = 1
	if len(ids) > 0 {
		e.ID = ids[len(ids)-1] + 1
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	e.Hash = Hash(content)
	e.Size = len(content)
	if err := os.WriteFile(s.path(e.ID, ".cfg"), []byte(content), 0o600); err != nil {
		return e, err
	}
	b, _ := json.MarshalIndent(e, "", "  ")
	if err := os.WriteFile(s.path(e.ID, ".json"), b, 0o600); err != nil {
		return e, err
	}
	if s.Keep > 0 && len(ids)+1 > s.Keep {
		for _, old := range ids[:len(ids)+1-s.Keep] {
			os.Remove(s.path(old, ".json"))
			os.Remove(s.path(old, ".cfg"))
		}
	}
	return e, nil
}

// List returns the newest entries first.
func (s *Store) List(limit int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.ids()
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for k := len(ids) - 1; k >= 0; k-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		e, err := s.readEntry(ids[k])
		if err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Store) readEntry(id int64) (Entry, error) {
	var e Entry
	b, err := os.ReadFile(s.path(id, ".json"))
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(b, &e)
	return e, err
}

// Get returns an entry and its content.
func (s *Store) Get(id int64) (Entry, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.readEntry(id)
	if err != nil {
		return e, "", fmt.Errorf("version %d not found", id)
	}
	b, err := os.ReadFile(s.path(id, ".cfg"))
	if err != nil {
		return e, "", err
	}
	return e, string(b), nil
}

// Latest returns the newest entry, if any.
func (s *Store) Latest() (*Entry, error) {
	l, err := s.List(1)
	if err != nil || len(l) == 0 {
		return nil, err
	}
	return &l[0], nil
}

// Previous returns the newest entry with an id lower than id.
func (s *Store) Previous(id int64) (*Entry, string, error) {
	s.mu.Lock()
	ids, err := s.ids()
	s.mu.Unlock()
	if err != nil {
		return nil, "", err
	}
	for k := len(ids) - 1; k >= 0; k-- {
		if ids[k] < id {
			e, c, err := s.Get(ids[k])
			if err != nil {
				return nil, "", err
			}
			return &e, c, nil
		}
	}
	return nil, "", nil
}
