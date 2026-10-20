// Package bibliohistory keeps a private git history of a project's biblio folder, outside the project's own repository.
package bibliohistory

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
)

// ErrNoHistory means the project's history repository has not been created yet.
var ErrNoHistory = errors.New("bibliohistory: no history recorded for this project yet")

// GitDir is the history repository of a project, keyed by its id in projects.json.
func GitDir(projectID string) string {
	return filepath.Join(config.Dir(), "history", projectID+".git")
}

// Repo is a bare repository used with the biblio folder as its work tree, so the project's own .git never sees it.
type Repo struct {
	gitDir   string
	workTree string
}

func New(gitDir, workTree string) *Repo {
	return &Repo{gitDir: gitDir, workTree: workTree}
}

func (r *Repo) Exists() bool {
	info, err := os.Stat(filepath.Join(r.gitDir, "HEAD"))
	return err == nil && !info.IsDir()
}

func (r *Repo) Init() error {
	if r.Exists() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.gitDir), 0o700); err != nil {
		return fmt.Errorf("bibliohistory: %w", err)
	}
	git, err := GitPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(git, append(isolation(), "init", "--quiet", "--bare", "--initial-branch=main", r.gitDir)...)
	cmd.Env = gitEnv(nil)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("bibliohistory: git init: %s", msg)
	}
	return nil
}

// The user's own git configuration is shut out so a setting such as signing, hooks or an excludes file can neither
// break a commit nor change what it records.
func isolation() []string {
	return []string{
		"-c", "user.name=lemongrass",
		"-c", "user.email=lemongrass@localhost",
		"-c", "commit.gpgsign=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.excludesFile=" + os.DevNull,
		"-c", "core.quotePath=false",
		"-c", "core.autocrlf=false",
		"-c", "gc.auto=0",
	}
}

func gitEnv(extra []string) []string {
	var env []string
	for _, kv := range os.Environ() {
		// Windows environment names are case-insensitive.
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	return append(env, extra...)
}

func (r *Repo) run(env []string, stdin []byte, args ...string) ([]byte, error) {
	git, err := GitPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"--git-dir", r.gitDir, "--work-tree", r.workTree}, isolation()...)
	cmd := exec.Command(git, append(full, args...)...)
	cmd.Env = gitEnv(env)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("bibliohistory: git %s: %s", args[0], msg)
	}
	return stdout.Bytes(), nil
}

func (r *Repo) head() (string, bool) {
	out, err := r.run(nil, nil, "rev-parse", "--quiet", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// Change is one path in a commit; OldPath is set only for a rename.
type Change struct {
	Status  string
	Path    string
	OldPath string
}

// Parses `--name-status -z` output, where a rename or copy carries its old path before its new one.
func parseNameStatus(out []byte) []Change {
	fields := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	var changes []Change
	for i := 0; i < len(fields); i++ {
		status := fields[i]
		if status == "" || i+1 >= len(fields) {
			continue
		}
		c := Change{Status: status[:1]}
		if (c.Status == "R" || c.Status == "C") && i+2 < len(fields) {
			c.OldPath, c.Path = fields[i+1], fields[i+2]
			i += 2
		} else {
			c.Path = fields[i+1]
			i++
		}
		changes = append(changes, c)
	}
	return changes
}

// Sessions names who wrote a snapshot's changes. It receives the changed paths relative to the work tree, and the time
// of the previous commit.
type Sessions func(paths []string, since time.Time) []string

const sessionTrailer = "Session"

// Replaced in tests to make commits with chosen dates.
var clock = time.Now

// Snapshot stages the whole work tree and commits it when it differs from the last commit, reporting whether it did.
func (r *Repo) Snapshot(sessions Sessions) (bool, error) {
	if info, err := os.Stat(r.workTree); err != nil || !info.IsDir() {
		return false, nil
	}
	if _, err := r.run(nil, nil, "add", "--all"); err != nil {
		return false, err
	}
	out, err := r.run(nil, nil, "diff", "--cached", "--name-status", "-M", "-z")
	if err != nil {
		return false, err
	}
	changes := parseNameStatus(out)
	if len(changes) == 0 {
		return false, nil
	}

	var who []string
	if head, ok := r.head(); ok && sessions != nil {
		if since, err := r.commitTime(head); err == nil {
			paths := make([]string, 0, len(changes))
			for _, c := range changes {
				paths = append(paths, c.Path)
			}
			who = sessions(paths, since)
		}
	}

	msg := fmt.Sprintf("%d changed\n", len(changes))
	if len(who) > 0 {
		msg += "\n"
		for _, s := range who {
			msg += sessionTrailer + ": " + s + "\n"
		}
	}
	date := clock().Format(time.RFC3339)
	env := []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	if _, err := r.run(env, []byte(msg), "commit", "--quiet", "--no-verify", "--file", "-"); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repo) commitTime(rev string) (time.Time, error) {
	out, err := r.run(nil, nil, "log", "-1", "--format=%ct", rev)
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("bibliohistory: commit time of %s: %w", rev, err)
	}
	return time.Unix(sec, 0), nil
}
