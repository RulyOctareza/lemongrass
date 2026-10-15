package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/hook"
	"github.com/faizalv/lemongrass/internal/session"
)

const replayUsage = `usage: lgrass replay [--session-id <id>] <held-call-id>

Runs a call a deny message held, so it is not resent. The call is checked again by every gate
first, and a deny prints its reason and keeps the call held. A held call runs once and expires
10 minutes after it was held. Bash and Write calls can be held.

  --session-id <id>   the session that holds the call, needed when the agent does not export
                      CLAUDE_CODE_SESSION_ID
`

func cmdReplay(args []string) {
	if signHelpRequested(args) {
		fmt.Print(replayUsage)
		return
	}
	id, sessionID, err := signRequest(args, "held call id")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, replayUsage)
		os.Exit(1)
	}
	if sessionID == "" {
		fmt.Fprintln(os.Stderr, "error: no session id is available. Pass --session-id when the agent does not export CLAUDE_CODE_SESSION_ID")
		os.Exit(1)
	}

	proj := currentProject()
	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	call, err := store.HeldCall(sessionID, id)
	if errors.Is(err, session.ErrNoHeldCall) {
		fmt.Fprintf(os.Stderr, "error: no held call %s for this session. It already ran, expired or was held by another session, so send the call again.\n", id)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	rc, err := hook.DecodeHeldCall(call)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	reason, err := hook.RecheckHeldCall(call)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if reason != "" {
		fmt.Fprintf(os.Stderr, "%s\n\nThe call did not run and is still held as %s. Once the deny above is resolved, run `lgrass replay %s` again instead of resending it.\n", reason, id, id)
		os.Exit(1)
	}
	dropped, err := store.DropHeldCall(sessionID, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !dropped {
		fmt.Fprintf(os.Stderr, "error: held call %s already ran.\n", id)
		os.Exit(1)
	}
	store.Close()
	os.Exit(runReplay(rc))
}

func runReplay(rc hook.ReplayCall) int {
	if rc.Tool == "Write" {
		if err := os.MkdirAll(filepath.Dir(rc.FilePath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(rc.FilePath, []byte(rc.Content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Printf("wrote %s\n", rc.FilePath)
		return 0
	}
	cmd := exec.Command("bash", "-c", rc.Command)
	cmd.Dir = rc.Cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
