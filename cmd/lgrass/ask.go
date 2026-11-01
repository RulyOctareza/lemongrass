// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"

	"gopkg.in/yaml.v3"
)

// Exit codes let a model that ran ask in the background tell the outcomes apart without parsing text.
const (
	askExitAnswered = 0
	askExitDeclined = 2
	askExitPending  = 3
)

// askFile is the YAML a model writes and passes by path, so the shell never sees a command or reason and backticks, $(...) and quotes stay literal.
type askFile struct {
	Type    string   `yaml:"type"`
	Tables  []string `yaml:"tables"`
	Reason  string   `yaml:"reason"`
	Message string   `yaml:"message"`
	Command string   `yaml:"command"`
}

func cmdAsk(args []string) {
	switch {
	case len(args) == 2 && args[0] == "show":
		requireAskTab()
		client := &agent.Client{SocketPath: agent.SocketPath()}
		a, err := client.AskResult(args[1], tabID)
		if err != nil {
			fail(err)
		}
		text, code := formatAsk(a)
		fmt.Println(text)
		os.Exit(code)
	case len(args) == 1 && !strings.HasPrefix(args[0], "-"):
		requireAskTab()
		os.Exit(fileAndWait(args[0], os.Stdout))
	default:
		fmt.Fprintln(os.Stderr, "usage: lgrass ask <path-to-ask.yaml> | lgrass ask show <ask-id>")
		os.Exit(1)
	}
}

func requireAskTab() {
	if tabID == "" {
		fmt.Fprintf(os.Stderr, "lgrass ask: asking needs a lemongrass tab, %s is not set\n", tabIDEnv)
		os.Exit(1)
	}
}

func fileAndWait(path string, out io.Writer) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(fmt.Errorf("lgrass ask: reading %s: %w", path, err))
	}
	req, err := parseAsk(data)
	if err != nil {
		fail(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fail(fmt.Errorf("lgrass ask: resolving the working directory: %w", err))
	}
	req.TabID = tabID
	req.Cwd = cwd
	req.Path = os.Getenv("PATH")

	client := &agent.Client{SocketPath: agent.SocketPath()}
	a, err := client.CreateAsk(req)
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(out, "Ask %s sent to the user. Waiting up to 30 minutes for the answer.\n", a.ID)
	a, err = client.WaitAsk(a.ID, tabID, vault.AskMaxWait)
	if err != nil {
		fail(err)
	}
	text, code := formatAsk(a)
	fmt.Fprintln(out, text)
	if a.Final() {
		// The vault nudges the tab with the outcome unless it hears it arrived; a failed ack only costs a repeated notice.
		_ = client.AckAsk(a.ID, tabID)
	}
	return code
}

// parseAsk reads one ask. Unknown fields and fields that belong to another type are errors, so a typo never files a different ask than the model meant.
func parseAsk(data []byte) (gatekeeper.AskRequest, error) {
	var f askFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return gatekeeper.AskRequest{}, errors.New("lgrass ask: the ask file is empty")
		}
		return gatekeeper.AskRequest{}, fmt.Errorf("lgrass ask: parsing the ask file: %w", err)
	}
	f.Type = strings.TrimSpace(f.Type)
	req := gatekeeper.AskRequest{Type: f.Type, Reason: strings.TrimSpace(f.Reason)}
	switch f.Type {
	case vault.AskDB:
		if f.Message != "" || f.Command != "" {
			return req, errors.New("lgrass ask: a db ask takes tables and reason only")
		}
		if len(f.Tables) == 0 {
			return req, errors.New("lgrass ask: a db ask needs tables, a list of the table names you need")
		}
		if req.Reason == "" {
			return req, errors.New("lgrass ask: a db ask needs a reason")
		}
		req.Tables = f.Tables
	case vault.AskRester:
		if len(f.Tables) > 0 || f.Command != "" {
			return req, errors.New("lgrass ask: a rester ask takes message and an optional reason only")
		}
		req.Message = strings.TrimSpace(f.Message)
		if req.Message == "" {
			return req, errors.New("lgrass ask: a rester ask needs a message saying which rester, methods and paths you need")
		}
	case vault.AskCommand:
		if len(f.Tables) > 0 || f.Message != "" {
			return req, errors.New("lgrass ask: a command ask takes command and reason only")
		}
		req.Command = strings.TrimSpace(f.Command)
		if req.Command == "" {
			return req, errors.New("lgrass ask: a command ask needs the exact command")
		}
		if req.Reason == "" {
			return req, errors.New("lgrass ask: a command ask needs a reason")
		}
	case "":
		return req, errors.New("lgrass ask: the ask file needs a type: db, rester or command")
	default:
		return req, fmt.Errorf("lgrass ask: unknown type %q, want db, rester or command", f.Type)
	}
	return req, nil
}

// formatAsk is what the model reads for an ask as it stands, with the exit code for it.
func formatAsk(a vault.Ask) (string, int) {
	show := "lgrass ask show " + a.ID
	switch a.State {
	case vault.AskAnswered:
		switch a.Type {
		case vault.AskDB:
			return fmt.Sprintf("Ask %s answered: the user created db channel %s. Use it with lgrass db %s --sql \"<statement>\".", a.ID, a.ShortID, a.ShortID), askExitAnswered
		case vault.AskRester:
			return fmt.Sprintf("Ask %s answered: the user created rester channel %s. Start with lgrass rester %s info.", a.ID, a.ShortID, a.ShortID), askExitAnswered
		}
		return formatCommandResult(a), askExitAnswered
	case vault.AskDeclined:
		note := strings.TrimSpace(a.Note)
		if note == "" {
			note = "no note given"
		}
		return fmt.Sprintf("Ask %s declined by the user: %s\nDo not ask again with a variation of the same request.", a.ID, note), askExitDeclined
	case vault.AskExpired:
		return fmt.Sprintf("Ask %s expired without an answer.", a.ID), askExitDeclined
	case vault.AskRunning:
		return fmt.Sprintf("Ask %s: the user is running the command and it has not finished. The result arrives as a notification; read it then with %s.", a.ID, show), askExitPending
	}
	return fmt.Sprintf("Ask %s has no answer yet. It stays open; the answer arrives as a notification, read it then with %s. Do not file it again.", a.ID, show), askExitPending
}

func formatCommandResult(a vault.Ask) string {
	r := a.Result
	if r == nil {
		return fmt.Sprintf("Ask %s answered.", a.ID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Ask %s: the user executed the command, exit code %d.", a.ID, r.ExitCode)
	if r.TimedOut {
		b.WriteString(" It hit the timeout the user set and was stopped.")
	}
	if r.Stopped {
		b.WriteString(" The user stopped it before it finished.")
	}
	if strings.TrimSpace(r.OutputTail) == "" {
		b.WriteString("\nIt printed nothing.")
	} else {
		b.WriteString("\nLast lines of its output:\n")
		b.WriteString(r.OutputTail)
	}
	return b.String()
}
