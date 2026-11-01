// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/vault"
)

// askFileTimeout matches the policy read: the hook must never hold a tool call up for long, so a slow daemon means the model files the ask itself.
const askFileTimeout = 500 * time.Millisecond

// dangerMessage is the refusal the model sees. A shell command refused by a rule in ask mode is filed as a command ask here, without waiting for the answer, and the refusal says the request was sent. Every other verdict keeps the guard's own message.
func dangerMessage(payload hookEvent, verdict guard.Verdict) string {
	return dangerMessageWith(gatekeeper.SocketPath(), payload, verdict)
}

func dangerMessageWith(socketPath string, payload hookEvent, verdict guard.Verdict) string {
	command, ok := askableCommand(payload, verdict)
	if !ok {
		return verdict.Message()
	}
	if holds, simple := simpleCommand(command); !simple {
		return askNotFiledMessage(verdict, holds)
	}
	client := &gatekeeper.Client{SocketPath: socketPath, Timeout: askFileTimeout}
	ask, err := client.CreateAsk(gatekeeper.AskRequest{
		Type: vault.AskCommand, TabID: payload.TabID, Cwd: payload.Cwd, Path: os.Getenv("PATH"),
		Command: command, Rule: verdict.RuleID,
	})
	if err != nil {
		return askFailedMessage(verdict, err)
	}
	return askFiledMessage(verdict, ask.ID)
}

// askableCommand is the shell command an ask-mode verdict refused. A verdict in deny mode, or on a file tool, has nothing the human could execute.
func askableCommand(payload hookEvent, verdict guard.Verdict) (string, bool) {
	if verdict.Mode == guard.ModeDeny || payload.ToolName != "Bash" || payload.TabID == "" {
		return "", false
	}
	var shell shellToolInput
	if err := json.Unmarshal(payload.ToolInput, &shell); err != nil || strings.TrimSpace(shell.Command) == "" {
		return "", false
	}
	return shell.Command, true
}

func ruleLabel(verdict guard.Verdict) string {
	return "rule " + verdict.RuleID + " (" + verdict.Description + ")"
}

func askFiledMessage(verdict guard.Verdict, askID string) string {
	msg := "lgrass: " + ruleLabel(verdict) + " needs the user's decision, so lemongrass sent the user a request to run this exact command, ask " + askID + ". Do not retry the call and do not file another ask for it. The user executes it or declines, and the answer arrives as a notification; read it then with lgrass ask show " + askID + "."
	if verdict.Detail != "" {
		msg += " " + verdict.Detail
	}
	return msg + " Run lgrass how ask request for how asks work."
}

func askNotFiledMessage(verdict guard.Verdict, holds string) string {
	msg := "lgrass: " + ruleLabel(verdict) + " needs the user's decision. lemongrass sends the user a request only for one simple command on its own, and this call holds " + holds + ", so no request was sent. Run the guarded command alone, as its own call with nothing chained around it, and the request is filed then; run the rest of the work as separate calls."
	if verdict.Detail != "" {
		msg += " " + verdict.Detail
	}
	return msg + " Run lgrass how ask request for how asks work."
}

func askFailedMessage(verdict guard.Verdict, err error) string {
	return "lgrass: " + ruleLabel(verdict) + " needs the user's decision, and sending the request failed (" + err.Error() + "). File it yourself with lgrass ask; run lgrass how ask request for how."
}
