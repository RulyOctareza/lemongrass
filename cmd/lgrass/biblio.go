package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/faizalv/lemongrass/internal/bibliohistory"
)

const (
	biblioLogDefault  = 20
	biblioLogMaxPaths = 5
	shortSession      = 8
)

func cmdBiblio(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass biblio <log|show> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "log":
		cmdBiblioLog(args[1:])
	case "show":
		cmdBiblioShow(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown biblio command: %s\n", args[0])
		os.Exit(1)
	}
}

func biblioRepo() (*bibliohistory.Repo, string) {
	proj := currentProject()
	return bibliohistory.New(bibliohistory.GitDir(proj.ID), filepath.Join(proj.Path, "biblio")), proj.Path
}

func biblioFail(err error) {
	if errors.Is(err, bibliohistory.ErrNoGit) {
		fmt.Fprintln(os.Stderr, "error: biblio history needs git, which is not installed")
	} else if errors.Is(err, bibliohistory.ErrNoHistory) {
		fmt.Fprintln(os.Stderr, "error: no biblio history yet; lemongrass records it while the app is running")
	} else {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(1)
}

func cmdBiblioLog(args []string) {
	limit := biblioLogDefault
	var path string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--limit" && i+1 < len(args):
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				fmt.Fprintln(os.Stderr, "error: --limit takes a positive number")
				os.Exit(1)
			}
			limit = n
			i++
		case path == "" && !strings.HasPrefix(args[i], "-"):
			path = args[i]
		default:
			fmt.Fprintln(os.Stderr, "usage: lgrass biblio log [<path>] [--limit N]")
			os.Exit(1)
		}
	}

	repo, projectPath := biblioRepo()
	rel, err := bibliohistory.RelPath(projectPath, path)
	if err != nil {
		biblioFail(err)
	}
	entries, err := repo.Log(rel, limit)
	if err != nil {
		biblioFail(err)
	}
	if len(entries) == 0 {
		if rel == "" {
			fmt.Println("no changes recorded yet.")
		} else {
			fmt.Printf("no changes recorded for %s.\n", rel)
		}
		return
	}
	for _, e := range entries {
		fmt.Println(formatBiblioEntry(e))
	}
}

// One line per commit, since it lands in a model's context: id, local time, sessions, then the paths it changed.
func formatBiblioEntry(e bibliohistory.Entry) string {
	sessions := "-"
	if len(e.Sessions) > 0 {
		short := make([]string, len(e.Sessions))
		for i, s := range e.Sessions {
			if len(s) > shortSession {
				s = s[:shortSession]
			}
			short[i] = s
		}
		sessions = strings.Join(short, ",")
	}
	var changes []string
	for i, c := range e.Changes {
		if i == biblioLogMaxPaths {
			changes = append(changes, fmt.Sprintf("and %d more", len(e.Changes)-i))
			break
		}
		if c.OldPath != "" {
			changes = append(changes, fmt.Sprintf("%s %s -> %s", c.Status, c.OldPath, c.Path))
		} else {
			changes = append(changes, c.Status+" "+c.Path)
		}
	}
	return fmt.Sprintf("%s  %s  %s  %s", e.Hash, e.Time.Local().Format("2006-01-02 15:04"), sessions, strings.Join(changes, ", "))
}

func cmdBiblioShow(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: lgrass biblio show <rev> <path>")
		os.Exit(1)
	}
	repo, projectPath := biblioRepo()
	rel, err := bibliohistory.RelPath(projectPath, args[1])
	if err != nil {
		biblioFail(err)
	}
	data, err := repo.Show(args[0], rel)
	if err != nil {
		biblioFail(err)
	}
	os.Stdout.Write(data)
}
