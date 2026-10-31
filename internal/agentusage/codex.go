// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// A rollout lives in the folder of the day its session started, so a session still running from earlier days is
	// found only if those folders are looked at too.
	codexDayFolders = 7
	codexFilesTried = 5
	// Codex writes a token_count event with its rate_limits after every turn, so the newest one sits near the end.
	codexTailBytes = 512 * 1024
)

type CodexRecord struct {
	Windows  []Window `json:"windows"`
	PlanType string   `json:"plan_type,omitempty"`
	SeenAt   int64    `json:"seen_at"`
}

type codexWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type codexLine struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type       string `json:"type"`
		RateLimits *struct {
			Primary   *codexWindow `json:"primary"`
			Secondary *codexWindow `json:"secondary"`
			PlanType  string       `json:"plan_type"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

func CodexHome(home string) string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".codex")
}

// ReadCodex returns the newest rate_limits snapshot Codex wrote into its rollouts, and ok false when there is none.
func ReadCodex(codexHome string) (CodexRecord, bool, error) {
	files, err := recentRollouts(filepath.Join(codexHome, "sessions"))
	if err != nil {
		return CodexRecord{}, false, fmt.Errorf("agentusage: listing Codex rollouts: %w", err)
	}
	for i, path := range files {
		if i == codexFilesTried {
			break
		}
		rec, ok, err := lastCodexSnapshot(path)
		if err != nil {
			return CodexRecord{}, false, fmt.Errorf("agentusage: reading %s: %w", path, err)
		}
		if ok {
			return rec, true, nil
		}
	}
	return CodexRecord{}, false, nil
}

func sortedDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

// recentRollouts lists the rollouts of the newest day folders (sessions/YYYY/MM/DD), newest modified first.
func recentRollouts(sessions string) ([]string, error) {
	var days []string
	years, err := sortedDirs(sessions)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, y := range years {
		months, _ := sortedDirs(filepath.Join(sessions, y))
		for _, m := range months {
			ds, _ := sortedDirs(filepath.Join(sessions, y, m))
			for _, d := range ds {
				days = append(days, filepath.Join(sessions, y, m, d))
				if len(days) == codexDayFolders {
					break
				}
			}
			if len(days) == codexDayFolders {
				break
			}
		}
		if len(days) == codexDayFolders {
			break
		}
	}

	type rollout struct {
		path string
		mod  time.Time
	}
	var all []rollout
	for _, day := range days {
		entries, _ := os.ReadDir(day)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			all = append(all, rollout{filepath.Join(day, e.Name()), info.ModTime()})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	paths := make([]string, len(all))
	for i, r := range all {
		paths[i] = r.path
	}
	return paths, nil
}

func lastCodexSnapshot(path string) (CodexRecord, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return CodexRecord{}, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return CodexRecord{}, false, err
	}
	offset := info.Size() - codexTailBytes
	if offset < 0 {
		offset = 0
	}
	tail, err := io.ReadAll(io.NewSectionReader(f, offset, info.Size()-offset))
	if err != nil {
		return CodexRecord{}, false, err
	}
	lines := bytes.Split(tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"rate_limits":{`)) {
			continue
		}
		var line codexLine
		if json.Unmarshal(lines[i], &line) != nil || line.Payload.Type != "token_count" || line.Payload.RateLimits == nil {
			continue
		}
		limits := line.Payload.RateLimits
		rec := CodexRecord{PlanType: limits.PlanType, SeenAt: info.ModTime().Unix()}
		if ts, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
			rec.SeenAt = ts.Unix()
		}
		for _, w := range []*codexWindow{limits.Primary, limits.Secondary} {
			if w != nil {
				rec.Windows = append(rec.Windows, Window{UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, WindowMinutes: w.WindowMinutes})
			}
		}
		if len(rec.Windows) > 0 {
			return rec, true, nil
		}
	}
	return CodexRecord{}, false, nil
}
