// Package auth: email magic links, browser sessions, and encryption of
// player API keys at rest.
//
// v1 has no mail sender. The magic link is printed to the server log (and
// returned to the caller so a dev UI can show it). Wire an SMTP Sender when
// there is more than one player.
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/internal/store"
)

const (
	linkTTL    = 15 * time.Minute
	sessionTTL = 30 * 24 * time.Hour
	CookieName = "fs_session"
)

// Sender delivers a magic link. LogSender prints it; an SMTP sender can
// replace it without touching anything else.
type Sender interface {
	SendMagicLink(ctx context.Context, email, url string) error
}

type LogSender struct{}

func (LogSender) SendMagicLink(_ context.Context, email, url string) error {
	log.Printf("MAGIC LINK for %s: %s", email, url)
	return nil
}

type Auth struct {
	Store   store.Store
	Sender  Sender
	BaseURL string // e.g. http://localhost:8080
	// DevEcho returns the link from Begin so the login page can show it.
	// Never enable in production.
	DevEcho bool
}

func token() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// NewID makes an opaque record id.
func NewID() string { return token()[:20] }

// Begin issues a magic link for an email. Returns the URL only when DevEcho.
func (a *Auth) Begin(ctx context.Context, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return "", errors.New("that does not look like an email address")
	}
	t := token()
	if err := a.Store.PutMagicLink(ctx, store.MagicLink{Token: t, Email: email, ExpiresAt: time.Now().Add(linkTTL)}); err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/login/%s", strings.TrimRight(a.BaseURL, "/"), t)
	if err := a.Sender.SendMagicLink(ctx, email, url); err != nil {
		return "", err
	}
	if a.DevEcho {
		return url, nil
	}
	return "", nil
}

// Complete consumes a magic link, creating the player if new, and returns
// a session token for the cookie.
func (a *Auth) Complete(ctx context.Context, linkToken string) (*store.Player, string, error) {
	email, err := a.Store.ConsumeMagicLink(ctx, linkToken)
	if err != nil {
		return nil, "", errors.New("this link is invalid or has expired")
	}
	p, err := a.Store.GetPlayerByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		p = &store.Player{ID: NewID(), Email: email, CreatedAt: time.Now()}
		if err := a.Store.PutPlayer(ctx, p); err != nil {
			return nil, "", err
		}
	} else if err != nil {
		return nil, "", err
	}
	st := token()
	if err := a.Store.PutSession(ctx, store.Session{Token: st, PlayerID: p.ID, ExpiresAt: time.Now().Add(sessionTTL)}); err != nil {
		return nil, "", err
	}
	return p, st, nil
}

// Resolve maps a session token to a player.
func (a *Auth) Resolve(ctx context.Context, sessionToken string) (*store.Player, error) {
	if sessionToken == "" {
		return nil, store.ErrNotFound
	}
	s, err := a.Store.GetSession(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	return a.Store.GetPlayer(ctx, s.PlayerID)
}

// ---------- API key encryption ----------

// Cipher encrypts player API keys with AES-256-GCM under a master key
// derived from FOUNDER_SIM_MASTER_KEY. One env var for v1; per-user
// derivation can come later without changing the stored shape.
type Cipher struct{ aead cipher.AEAD }

func NewCipher(masterKey string) (*Cipher, error) {
	if len(masterKey) < 16 {
		return nil, errors.New("master key must be at least 16 characters")
	}
	k := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, c.aead.Seal(nil, nonce, []byte(plain), nil)...), nil
}

func (c *Cipher) Decrypt(blob []byte) (string, error) {
	if len(blob) == 0 {
		return "", nil
	}
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return "", errors.New("ciphertext too short")
	}
	out, err := c.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
