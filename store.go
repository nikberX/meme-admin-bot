package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Store struct {
	baseDir   string
	mediaDir  string
	statePath string

	mu     sync.Mutex
	drafts map[string]Draft
}

func NewStore(baseDir string) (*Store, error) {
	baseDir = filepath.Clean(baseDir)
	mediaDir := filepath.Join(baseDir, "media")
	statePath := filepath.Join(baseDir, "drafts.json")

	for _, dir := range []string{baseDir, mediaDir, filepath.Join(baseDir, "tmp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	s := &Store{
		baseDir:   baseDir,
		mediaDir:  mediaDir,
		statePath: statePath,
		drafts:    make(map[string]Draft),
	}

	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) MediaPath(name string) string {
	return filepath.Join(s.mediaDir, filepath.Base(name))
}

func (s *Store) SaveDraft(d Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drafts[d.ID] = d
	return s.persistLocked()
}

func (s *Store) GetDraft(id string) (Draft, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	return d, ok
}

func (s *Store) ListDrafts(ownerUserID int64) []Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Draft, 0, len(s.drafts))
	for _, d := range s.drafts {
		if d.OwnerUserID == ownerUserID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ListAllDrafts() []Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Draft, 0, len(s.drafts))
	for _, d := range s.drafts {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) NextDraftID() string {
	return fmt.Sprintf("m%v", time.Now().UnixNano())
}

func (s *Store) load() error {
	payload, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state: %w", err)
	}
	var drafts []Draft
	if err := json.Unmarshal(payload, &drafts); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}
	for _, d := range drafts {
		s.drafts[d.ID] = d
	}
	return nil
}

func (s *Store) persistLocked() error {
	drafts := make([]Draft, 0, len(s.drafts))
	for _, d := range s.drafts {
		drafts = append(drafts, d)
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].CreatedAt.Before(drafts[j].CreatedAt)
	})
	payload, err := json.MarshalIndent(drafts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.statePath, payload, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
