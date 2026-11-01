// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

// A record nobody has refreshed for this long belongs to a tab that is gone, and is removed on the next write.
const contextKeep = 7 * 24 * time.Hour

// A tab id becomes a file name, so it is held to the characters lemongrass tab ids use.
var tabIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Context is one tab's context window use as of its latest statusline payload.
type Context struct {
	UsedTokens   int64 `json:"used_tokens"`
	WindowTokens int64 `json:"window_tokens"`
	SeenAt       int64 `json:"seen_at"`
}

func ContextDir(home string) string {
	return filepath.Join(Dir(home), "context")
}

func contextPath(home, tabID string) string {
	return filepath.Join(ContextDir(home), tabID+".json")
}

// RecordContext saves the context_window block of one statusline payload for the tab. Used tokens are the latest
// request's input, cache writes and cache reads, which is what the next request carries. A payload without usage,
// which Claude Code sends before the first response, records nothing, and an unchanged figure is not rewritten,
// because the statusline runs on every update while Claude streams and the app watches these files.
func RecordContext(home, tabID string, payload []byte, now time.Time) error {
	if !tabIDPattern.MatchString(tabID) {
		return nil
	}
	var p struct {
		ContextWindow *struct {
			Size         int64 `json:"context_window_size"`
			CurrentUsage *struct {
				Input         int64 `json:"input_tokens"`
				CacheCreation int64 `json:"cache_creation_input_tokens"`
				CacheRead     int64 `json:"cache_read_input_tokens"`
			} `json:"current_usage"`
		} `json:"context_window"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("agentusage: parsing statusline payload: %w", err)
	}
	cw := p.ContextWindow
	if cw == nil || cw.Size <= 0 || cw.CurrentUsage == nil {
		return nil
	}
	u := cw.CurrentUsage
	rec := Context{UsedTokens: u.Input + u.CacheCreation + u.CacheRead, WindowTokens: cw.Size, SeenAt: now.Unix()}
	path := contextPath(home, tabID)
	if old, err := readContext(path); err == nil && old.UsedTokens == rec.UsedTokens && old.WindowTokens == rec.WindowTokens {
		return nil
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := agentconf.WriteAtomic(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	pruneContexts(home, now)
	return nil
}

func pruneContexts(home string, now time.Time) {
	entries, err := os.ReadDir(ContextDir(home))
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !strings.HasSuffix(e.Name(), ".json") || now.Sub(info.ModTime()) < contextKeep {
			continue
		}
		os.Remove(filepath.Join(ContextDir(home), e.Name()))
	}
}

func readContext(path string) (Context, error) {
	var rec Context
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("agentusage: parsing %s: %w", path, err)
	}
	return rec, nil
}

// ReadContexts returns every recorded tab's context by tab id. A file that cannot be read is skipped, since a record
// being replaced mid-read is rewritten again on the tab's next response.
func ReadContexts(home string) (map[string]Context, error) {
	out := map[string]Context{}
	entries, err := os.ReadDir(ContextDir(home))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		tabID, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !tabIDPattern.MatchString(tabID) {
			continue
		}
		if rec, err := readContext(filepath.Join(ContextDir(home), e.Name())); err == nil {
			out[tabID] = rec
		}
	}
	return out, nil
}
