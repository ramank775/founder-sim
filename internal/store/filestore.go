package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileStore keeps one JSON file per record under dir:
//
//	players/<id>.json      runs/<id>.json
//	links/<token>.json     sessions/<token>.json
//	email-index.json       (email -> player id)
//	bridge-index.json      (bridge token -> player id)
//
// Writes are atomic (temp file + rename). A single mutex serialises all
// access; this is a side project's persistence, not a database.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

func OpenFileStore(dir string) (*FileStore, error) {
	for _, sub := range []string{"players", "runs", "links", "sessions"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) Close() error { return nil }

func safeName(s string) string {
	// ids and tokens are ours (hex/alnum); refuse anything else defensively
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return ""
		}
	}
	return s
}

func (s *FileStore) path(kind, id string) (string, error) {
	n := safeName(id)
	if n == "" {
		return "", fmt.Errorf("invalid id %q", id)
	}
	return filepath.Join(s.dir, kind, n+".json"), nil
}

func (s *FileStore) read(kind, id string, v any) error {
	p, err := s.path(kind, id)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (s *FileStore) write(kind, id string, v any) error {
	p, err := s.path(kind, id)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *FileStore) remove(kind, id string) error {
	p, err := s.path(kind, id)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// ---- indexes ----

func (s *FileStore) loadIndexFile(name string) map[string]string {
	idx := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(s.dir, name))
	if err == nil {
		_ = json.Unmarshal(raw, &idx)
	}
	return idx
}

func (s *FileStore) saveIndexFile(name string, idx map[string]string) error {
	raw, _ := json.MarshalIndent(idx, "", " ")
	p := filepath.Join(s.dir, name)
	if err := os.WriteFile(p+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

func (s *FileStore) loadIndex() map[string]string { return s.loadIndexFile("email-index.json") }
func (s *FileStore) saveIndex(i map[string]string) error {
	return s.saveIndexFile("email-index.json", i)
}

// ---- players ----

func (s *FileStore) GetPlayerByEmail(ctx context.Context, email string) (*Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.loadIndex()[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return nil, ErrNotFound
	}
	var p Player
	if err := s.read("players", id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *FileStore) GetPlayer(ctx context.Context, id string) (*Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p Player
	if err := s.read("players", id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *FileStore) PutPlayer(ctx context.Context, p *Player) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write("players", p.ID, p); err != nil {
		return err
	}
	idx := s.loadIndex()
	idx[strings.ToLower(strings.TrimSpace(p.Email))] = p.ID
	if err := s.saveIndex(idx); err != nil {
		return err
	}
	// Bridge token index: drop any old token for this player, add the current.
	b := s.loadIndexFile("bridge-index.json")
	for tok, pid := range b {
		if pid == p.ID && tok != p.BridgeToken {
			delete(b, tok)
		}
	}
	if p.BridgeToken != "" {
		b[p.BridgeToken] = p.ID
	}
	return s.saveIndexFile("bridge-index.json", b)
}

func (s *FileStore) PlayerIDForBridgeToken(ctx context.Context, token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if safeName(token) == "" {
		return "", ErrNotFound
	}
	pid, ok := s.loadIndexFile("bridge-index.json")[token]
	if !ok {
		return "", ErrNotFound
	}
	return pid, nil
}

// ---- runs ----

func (s *FileStore) GetRun(ctx context.Context, id string) (*RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r RunRecord
	if err := s.read("runs", id, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *FileStore) PutRun(ctx context.Context, r *RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.UpdatedAt = time.Now()
	return s.write("runs", r.Run.ID, r)
}

func (s *FileStore) ListRuns(ctx context.Context, playerID string) ([]RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "runs"))
	if err != nil {
		return nil, err
	}
	var out []RunSummary
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var r RunRecord
		if err := s.read("runs", strings.TrimSuffix(e.Name(), ".json"), &r); err != nil || r.Run == nil {
			continue
		}
		if r.PlayerID != playerID {
			continue
		}
		out = append(out, RunSummary{ID: r.Run.ID, Idea: r.Run.Player.Idea, Day: r.Run.CurrentDay, Status: r.Run.Status, StartedAt: r.Run.StartedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}

// ---- auth ----

func (s *FileStore) PutMagicLink(ctx context.Context, m MagicLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write("links", m.Token, m)
}

func (s *FileStore) ConsumeMagicLink(ctx context.Context, token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var m MagicLink
	if err := s.read("links", token, &m); err != nil {
		return "", err
	}
	_ = s.remove("links", token)
	if time.Now().After(m.ExpiresAt) {
		return "", ErrNotFound
	}
	return m.Email, nil
}

func (s *FileStore) PutSession(ctx context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write("sessions", sess.Token, sess)
}

func (s *FileStore) GetSession(ctx context.Context, token string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sess Session
	if err := s.read("sessions", token, &sess); err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.remove("sessions", token)
		return nil, ErrNotFound
	}
	return &sess, nil
}

func (s *FileStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remove("sessions", token)
}

var _ Store = (*FileStore)(nil)
