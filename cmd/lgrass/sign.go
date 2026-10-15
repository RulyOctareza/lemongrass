package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/hook"
	"github.com/faizalv/lemongrass/internal/session"
)

const signUsage = `usage: lgrass sign [--session-id <id>] <word>

Signs a word for the current session. A deny message names the word to sign. A sign prints the
word's pledge, the rule it stands for, and satisfies the deny until the word's TTL
expires. A word that is not registered is rejected.

  --session-id <id>   the session to sign for, needed when the agent does not export
                      CLAUDE_CODE_SESSION_ID
`

// The model-facing side of the prerequisite gate: satisfies a registered word's PreToolUse deny for this session, for that word's own TTL, and answers with its pledge.
func cmdSign(args []string) {
	if signHelpRequested(args) {
		fmt.Print(signUsage)
		return
	}
	word, sessionID, err := signRequest(args, "word")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, signUsage)
		os.Exit(1)
	}
	proj := currentProject()
	signable, ok, err := session.ResolveSignable(proj.Path, word)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown word %q\n", word)
		os.Exit(1)
	}
	if sessionID == "" {
		fmt.Fprintln(os.Stderr, "error: no session id is available. Pass --session-id when the agent does not export CLAUDE_CODE_SESSION_ID")
		os.Exit(1)
	}

	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Sign(sessionID, signable.ID); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(signAcknowledgement(signable))
	if signable.ID == session.MemoryWriteWord {
		printHeldCalls(store, sessionID, proj.Path)
	}
}

func printHeldCalls(store *session.Store, sessionID, projectPath string) {
	calls, err := store.HeldCalls(sessionID)
	if err != nil {
		return
	}
	for _, call := range calls {
		fmt.Println()
		fmt.Println(hook.DescribeHeldCall(call, projectPath))
	}
}

func signHelpRequested(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func signAcknowledgement(s session.Signable) string {
	if s.Pledge == "" {
		return "signed " + s.ID + "."
	}
	return "signed " + s.ID + ". " + s.Pledge
}

func signRequest(args []string, noun string) (word, sessionID string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session-id":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", errors.New("missing session id")
			}
			sessionID = args[i]
		default:
			if word != "" {
				return "", "", errors.New("more than one " + noun)
			}
			word = args[i]
		}
	}
	if word == "" {
		return "", "", errors.New("missing " + noun)
	}
	if sessionID == "" {
		sessionID = os.Getenv("CLAUDE_CODE_SESSION_ID")
	}
	return word, sessionID, nil
}
