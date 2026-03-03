package backend

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	// TypeCodexTmux is the default tmux-based Codex backend.
	TypeCodexTmux = "codex-tmux"
	// TypeClaudeCode reserved for future Claude backend support.
	TypeClaudeCode = "claude-code"
	// TypeOpenAIAPI reserved for future API-driven backend support.
	TypeOpenAIAPI = "openai-api"
)

// BuildOptions contains shared backend construction options.
type BuildOptions struct {
	Binary        string
	BypassSandbox bool
}

// Factory creates a backend implementation from options.
type Factory func(opts BuildOptions) (AgentBackend, error)

// Registry maps backend type names to constructors.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

var defaultRegistry = NewRegistry()

// NewRegistry creates a backend registry with built-in backends.
func NewRegistry() *Registry {
	r := &Registry{
		factories: make(map[string]Factory),
	}
	_ = r.Register(TypeCodexTmux, func(opts BuildOptions) (AgentBackend, error) {
		return NewCodexBackend(opts.Binary, opts.BypassSandbox), nil
	})
	return r
}

// Register adds a backend factory.
func (r *Registry) Register(backendType string, factory Factory) error {
	if r == nil {
		return fmt.Errorf("backend registry is nil")
	}
	if factory == nil {
		return fmt.Errorf("backend factory is nil")
	}
	key := normalizeBackendType(backendType)
	if key == "" {
		return fmt.Errorf("backend type is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[key]; exists {
		return fmt.Errorf("backend type %q already registered", key)
	}
	r.factories[key] = factory
	return nil
}

// Build constructs a backend by type.
func (r *Registry) Build(backendType string, opts BuildOptions) (AgentBackend, error) {
	if r == nil {
		return nil, fmt.Errorf("backend registry is nil")
	}

	key := normalizeBackendType(backendType)
	if key == "" {
		key = TypeCodexTmux
	}

	r.mu.RLock()
	factory, ok := r.factories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported backend type %q (supported: %s)", key, strings.Join(r.supportedTypes(), ", "))
	}

	return factory(opts)
}

// Build constructs a backend via the default registry.
func Build(backendType string, opts BuildOptions) (AgentBackend, error) {
	return defaultRegistry.Build(backendType, opts)
}

func (r *Registry) supportedTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.factories))
	for k := range r.factories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func normalizeBackendType(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
