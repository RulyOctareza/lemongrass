// Package node serves granted books, scratchpad tasks, laws and handovers read-only from ~/.lemongrass/node.json, which only the Electron app writes.
package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
)

// Config mirrors ui/src/main/node.ts's shape in ~/.lemongrass/node.json.
type Config struct {
	Projects []ProjectAlias `json:"projects"`
	Grants   []Grant        `json:"grants"`
	Tunnel   Tunnel         `json:"tunnel"`
}

// ProjectAlias names a registered project the way consumers see it, unique within the node.
type ProjectAlias struct {
	Alias     string `json:"alias"`
	ProjectID string `json:"projectId"`
}

// Grant is one person's access, identified to the node only by the hash of its token.
type Grant struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Shares    []Share   `json:"shares"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"createdAt"`
	TokenHash string    `json:"tokenHash"`
}

// Share lists what a grant may read in one project: books by stable identity, tasks by slug, laws and handovers by file name without .md.
type Share struct {
	Alias     string   `json:"alias"`
	Books     []string `json:"books"`
	Tasks     []string `json:"tasks"`
	Laws      []string `json:"laws,omitempty"`
	Handovers []string `json:"handovers,omitempty"`
}

// Tunnel is on while Until is in the future.
type Tunnel struct {
	Provider  string    `json:"provider"`
	Authtoken string    `json:"authtoken,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	LocalPort int       `json:"localPort,omitempty"`
	Until     time.Time `json:"until"`
}

func ConfigPath() string {
	return filepath.Join(config.Dir(), "node.json")
}

// LoadConfig reads node.json, returning an empty config when the file does not exist.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("node: reading %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("node: parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Store rereads node.json whenever its mtime or size changes, so a revoke lands even when the reload hint is lost.
type Store struct {
	path string

	mu      sync.Mutex
	cfg     Config
	modTime time.Time
	size    int64
	loaded  bool
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Config returns the current config, reloading it first when the file changed.
func (s *Store) Config() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var modTime time.Time
	var size int64
	info, err := os.Stat(s.path)
	switch {
	case err == nil:
		modTime, size = info.ModTime(), info.Size()
	case !os.IsNotExist(err):
		return Config{}, fmt.Errorf("node: reading %s: %w", s.path, err)
	}
	if s.loaded && modTime.Equal(s.modTime) && size == s.size {
		return s.cfg, nil
	}
	cfg, err := LoadConfig(s.path)
	if err != nil {
		return Config{}, err
	}
	s.cfg, s.modTime, s.size, s.loaded = cfg, modTime, size, true
	return cfg, nil
}

// Reload forces the next Config call to read the file again.
func (s *Store) Reload() {
	s.mu.Lock()
	s.loaded = false
	s.mu.Unlock()
}
