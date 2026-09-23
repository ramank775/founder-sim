// Package store persists players, runs and plugin state.
//
// v1 ships a file store (one JSON document per record under a data dir) so
// the binary has zero dependencies. The Store interface is the seam: a
// SQLite implementation (modernc.org/sqlite, no cgo) slots in behind it
// without touching callers. The file store is also the reference for what
// the SQLite one must do, and store_test.go runs against any implementation.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/ramank775/founder-sim/engine"
)

var ErrNotFound = errors.New("not found")

// Player is an account. Email is the identity; the API key is encrypted
// at rest by the store's Cipher before it is written.
type Player struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	// LLM config. APIKeyEnc is ciphertext; the plaintext never hits disk.
	LLMBaseURL  string `json:"llm_base_url"`
	LLMModel    string `json:"llm_model"`
	LLMJSONMode bool   `json:"llm_json_mode"`
	APIKeyEnc   []byte `json:"api_key_enc,omitempty"`
	// BridgeToken authenticates the player's founder-sim-bridge. Empty = none issued.
	BridgeToken string `json:"bridge_token,omitempty"`
	// Learned carries across runs: checklist items the player has met.
	Learned []string `json:"learned"`
}

// RunRecord is a run plus the state each enabled plugin keeps for it.
type RunRecord struct {
	PlayerID    string                       `json:"player_id"`
	Run         *engine.Run                  `json:"run"`
	PluginState map[string]map[string][]byte `json:"plugin_state"` // plugin -> key -> blob; key "" is the default
	UpdatedAt   time.Time                    `json:"updated_at"`
}

// RunSummary is what the run list shows.
type RunSummary struct {
	ID        string
	Idea      string
	Day       int
	Status    string
	StartedAt int64
}

// MagicLink is a one-shot login token.
type MagicLink struct {
	Token     string
	Email     string
	ExpiresAt time.Time
}

// Session is a logged-in browser.
type Session struct {
	Token     string
	PlayerID  string
	ExpiresAt time.Time
}

// Store is everything the app needs to remember.
type Store interface {
	// Players
	GetPlayerByEmail(ctx context.Context, email string) (*Player, error)
	GetPlayer(ctx context.Context, id string) (*Player, error)
	PutPlayer(ctx context.Context, p *Player) error
	// PlayerIDForBridgeToken satisfies bridge.TokenResolver.
	PlayerIDForBridgeToken(ctx context.Context, token string) (string, error)

	// Runs
	GetRun(ctx context.Context, id string) (*RunRecord, error)
	PutRun(ctx context.Context, r *RunRecord) error
	ListRuns(ctx context.Context, playerID string) ([]RunSummary, error)

	// Auth
	PutMagicLink(ctx context.Context, m MagicLink) error
	// ConsumeMagicLink returns the email and deletes the link. Expired or
	// unknown tokens return ErrNotFound.
	ConsumeMagicLink(ctx context.Context, token string) (string, error)
	PutSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, token string) (*Session, error)
	DeleteSession(ctx context.Context, token string) error

	Close() error
}
