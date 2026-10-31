// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"fmt"
	"time"
)

type ReportWindow struct {
	Label       string  `json:"label"`
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at"`
	// Reset is true once resets_at has passed: the agent has not reported since, but the window is known to be empty.
	Reset bool `json:"reset"`
}

type AccountUsage struct {
	Account string         `json:"account"`
	Windows []ReportWindow `json:"windows"`
	SeenAt  int64          `json:"seen_at"`
}

type CodexUsage struct {
	Windows  []ReportWindow `json:"windows"`
	PlanType string         `json:"plan_type,omitempty"`
	SeenAt   int64          `json:"seen_at"`
}

type Report struct {
	Claude []AccountUsage `json:"claude"`
	Codex  *CodexUsage    `json:"codex"`
	Errors []string       `json:"errors,omitempty"`
}

func Label(minutes int) string {
	switch {
	case minutes <= 0:
		return "?"
	case minutes%1440 == 0:
		return fmt.Sprintf("%dd", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("%dh", minutes/60)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

func reportWindow(w Window, now time.Time) ReportWindow {
	r := ReportWindow{Label: Label(w.WindowMinutes), UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt}
	if w.ResetsAt > 0 && w.ResetsAt <= now.Unix() {
		r.UsedPercent = 0
		r.Reset = true
	}
	return r
}

// Collect reads every account's last statusline record and the newest Codex snapshot. An account with no record yet,
// and Codex with no rollout, are left out; a read failure is reported beside the rest instead of failing them.
func Collect(home, codexHome string, accounts []string, now time.Time) Report {
	report := Report{Claude: []AccountUsage{}}
	for _, account := range accounts {
		rec, ok, err := ReadClaude(home, account)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
			continue
		}
		if !ok {
			continue
		}
		usage := AccountUsage{Account: account, Windows: []ReportWindow{}, SeenAt: rec.SeenAt}
		for _, w := range []*Window{rec.FiveHour, rec.SevenDay} {
			if w != nil {
				usage.Windows = append(usage.Windows, reportWindow(*w, now))
			}
		}
		report.Claude = append(report.Claude, usage)
	}
	rec, ok, err := ReadCodex(codexHome)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else if ok {
		usage := &CodexUsage{Windows: []ReportWindow{}, PlanType: rec.PlanType, SeenAt: rec.SeenAt}
		for _, w := range rec.Windows {
			usage.Windows = append(usage.Windows, reportWindow(w, now))
		}
		report.Codex = usage
	}
	return report
}
