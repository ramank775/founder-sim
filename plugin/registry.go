package plugin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

var ErrNoSuchTool = errors.New("no such tool")

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidateManifest enforces the naming and protocol rules.
func ValidateManifest(m Manifest) error {
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("plugin name %q must match %s", m.Name, nameRe)
	}
	if m.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("plugin %s speaks protocol %q, engine speaks %q", m.Name, m.ProtocolVersion, ProtocolVersion)
	}
	for _, c := range m.Capabilities {
		switch c {
		case CapTools, CapHooks, CapContent, CapLLMTools:
		default:
			return fmt.Errorf("plugin %s: unknown capability %q", m.Name, c)
		}
	}
	return nil
}

// CallTimeout bounds every plugin call, built in or remote.
const CallTimeout = 10 * time.Second

// Registry holds the plugins an engine instance knows about, by name.
// Whether a plugin is *enabled* for a run is per-run (engine.Run.Plugins).
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]entry
}

type entry struct {
	p Plugin
	m Manifest
}

func NewRegistry() *Registry { return &Registry{plugins: map[string]entry{}} }

// Register fetches and validates the manifest, then adds the plugin.
func (r *Registry) Register(ctx context.Context, p Plugin) (Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	m, err := p.Manifest(ctx)
	if err != nil {
		return m, err
	}
	if err := ValidateManifest(m); err != nil {
		return m, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.plugins[m.Name]; dup {
		return m, fmt.Errorf("plugin %s already registered", m.Name)
	}
	r.plugins[m.Name] = entry{p: p, m: m}
	return m, nil
}

// Get returns a plugin and its manifest.
func (r *Registry) Get(name string) (Plugin, Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.plugins[name]
	return e.p, e.m, ok
}

// Manifests lists everything registered, sorted by name.
func (r *Registry) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Manifest, 0, len(r.plugins))
	for _, e := range r.plugins {
		out = append(out, e.m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
