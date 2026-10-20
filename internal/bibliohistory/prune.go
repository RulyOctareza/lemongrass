package bibliohistory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Prune drops every commit older than cutoff except the newest of them, which becomes the root, so the history still
// holds the full state at the cutoff and every change after it. It reports how many commits were dropped.
//
// The later commits are rebuilt from their own trees with commit-tree rather than replayed with rebase, because rebase
// checks out into the work tree, and the work tree is the live biblio folder. Every commit is a full snapshot, so no
// merge is needed. Rewriting is safe only because the repository has no remote and nothing keeps commit ids.
func (r *Repo) Prune(cutoff time.Time) (int, error) {
	head, ok := r.head()
	if !ok {
		return 0, nil
	}
	out, err := r.run(nil, nil, "rev-list", "-1", "--before="+cutoff.UTC().Format(time.RFC3339), "HEAD")
	if err != nil {
		return 0, err
	}
	base := strings.TrimSpace(string(out))
	if base == "" {
		return 0, nil
	}
	out, err = r.run(nil, nil, "rev-list", "--count", base)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("bibliohistory: counting commits: %w", err)
	}
	if n <= 1 {
		return 0, nil
	}

	out, err = r.run(nil, nil, "rev-list", "--reverse", base+"..HEAD")
	if err != nil {
		return 0, err
	}
	commits := append([]string{base}, strings.Fields(string(out))...)
	parent := ""
	for _, c := range commits {
		if parent, err = r.recommit(c, parent); err != nil {
			return 0, err
		}
	}
	if _, err := r.run(nil, nil, "update-ref", "HEAD", parent, head); err != nil {
		return 0, err
	}
	if _, err := r.run(nil, nil, "reflog", "expire", "--expire=now", "--all"); err != nil {
		return 0, err
	}
	if _, err := r.run(nil, nil, "gc", "--quiet", "--prune=now"); err != nil {
		return 0, err
	}
	return n - 1, nil
}

// Rebuilds commit c on parent, or as a root when parent is empty, keeping its tree, message and dates.
func (r *Repo) recommit(c, parent string) (string, error) {
	out, err := r.run(nil, nil, "log", "-1", "--format=%aI%x00%cI%x00%B", c)
	if err != nil {
		return "", err
	}
	meta := strings.SplitN(string(out), "\x00", 3)
	if len(meta) != 3 {
		return "", fmt.Errorf("bibliohistory: reading commit %s", c)
	}
	args := []string{"commit-tree", c + "^{tree}"}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-F", "-")
	env := []string{"GIT_AUTHOR_DATE=" + meta[0], "GIT_COMMITTER_DATE=" + meta[1]}
	out, err = r.run(env, []byte(strings.TrimRight(meta[2], "\n")+"\n"), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
