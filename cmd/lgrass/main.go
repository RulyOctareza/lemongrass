package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/cmd/lgrass/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	tabID = os.Getenv(tabIDEnv)

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Version)
	case "--help", "-h", "help":
		usage()
	case "thread":
		cmdThread(os.Args[2:])
	case "workgroup":
		cmdWorkgroup(os.Args[2:])
	case "session":
		cmdSession(os.Args[2:])
	case "db":
		cmdDb(os.Args[2:])
	case "rester":
		cmdRester(os.Args[2:])
	case "tips":
		cmdTips(os.Args[2:])
	case "sign":
		cmdSign(os.Args[2:])
	case "replay":
		cmdReplay(os.Args[2:])
	case "how":
		cmdHow(os.Args[2:])
	case "biblio":
		cmdBiblio(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`lgrass -- lemongrass's agent-invoked CLI

COMMANDS
  thread create "<title>" "<content>"
                                     Opens a thread with content as its first message and prints its id;
                                     content may be - for stdin or --file <path>, capped at 2000 characters
  thread post <thread-id> "<content>"
                                     Adds a message; !>>tab-id<<! in content mentions a tab by id or by
                                     a unique prefix of at least 8 characters
  thread read <thread-id> [--all] [--before <message-id>] [--limit N]
                                     Reads a thread newest first, 10 messages a page; a lemongrass
                                     tab sees only what it has not read unless --all is given
  thread list [--limit N]           Threads in this project, most recently active first

  workgroup create <path-to-config>
                                     Declares a workgroup from a YAML or JSON config and asks the human to approve it in the
                                     lemongrass app; approved thinkers open as tabs next to this one, and this tab is the leader
  workgroup add <path-to-config>   Leader only: proposes more thinkers for this tab's live workgroup from a config that lists
                                     only thinkers; once the human approves they open as tabs and join the group thread
  workgroup join --token <token> [--json]
                                     Run by the lemongrass app when the human adds an open tab to a live workgroup as a
                                     collaborator; the token comes only from the human's approval in the app
  workgroup disband <id>            Ends a workgroup; only its leader may. Tabs stay open and the thread stays readable
  workgroup thread [--all] [--before <message-id>] [--limit N]
                                     Reads this tab's workgroup thread, post to it with thread post
  workgroup list [--json]           Live workgroups in this project with their members and tab ids; needs no tab

  tips add "<message>"              Add a project-local custom tip, surfaced alongside the
                                     built-in ones on the periodic PostToolUse nudge
  tips list                         List this project's custom tips, with their ids
  tips remove <id>                  Remove a custom tip by id

  sign [--session-id <id>] <word>   Signs a word a deny message names for the current
                                     session until its TTL expires, and prints the word's pledge;
                                     an unregistered word is rejected

  replay [--session-id <id>] <held-call-id>
                                     Runs a call a deny message held instead of resending it, after
                                     every gate checks it again; a held call runs once and expires

  how [<domain> [<action> ...]]     Prints only the instructions for the step at hand; with no action it lists
                                     what exists. how task close <slug> picks the light or full close from
                                     the task's handover, child tasks and PRD size

  biblio log [<path>] [--limit N]   Changes recorded in this project's biblio history, newest first, 20 by default:
                                     id, time, sessions that wrote them, paths. A path is a file or folder relative
                                     to biblio and is followed back across moves such as archiving
  biblio show <rev> <path>          Prints a file as it was at a change id from biblio log, using the path it had then

  session list                      Other tabs open in this project, with short tab id, vendor and shell title
  session tabs list                 Tab id to session id map for this project, as JSON

  db <short-id> --sql "<statement>" [--dry-run|--commit]
                                     Model-facing: runs a statement against the database a channel
                                     grants access to, through the running agent and vault daemons.
                                     A read (SELECT/SHOW/DESCRIBE/EXPLAIN) runs immediately; a write
                                     (INSERT/UPDATE/DELETE) requires exactly one of --dry-run
                                     (rolled back, reports the row count) or --commit (persists)

  rester <short-id> info            Model-facing: prints a channel's base URL, expiry, allowed
                                     methods and users
  rester <short-id> users           Model-facing: prints a channel's users with their tags
  rester <short-id> <get|post|put|patch|delete|head|options> <path> [--user <name>] [--body '<json>']
                                     Model-facing: proxies one HTTP call through the channel's
                                     domain as user, through the running agent and vault daemons

  version                           Print version
`)
}
