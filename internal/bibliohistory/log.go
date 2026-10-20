package bibliohistory

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Entry struct {
	Hash     string
	Time     time.Time
	Sessions []string
	// Only the changes under the path asked for, each with the path it had at this commit.
	Changes []Change
}

// Log lists the commits that touched path, a file or folder relative to the work tree, newest first, or every commit
// when path is empty. It follows path back across moves such as archiving: when the oldest commit for a path is the one
// that moved it there, the log carries on under the path it had before.
func (r *Repo) Log(path string, limit int) ([]Entry, error) {
	if _, err := GitPath(); err != nil {
		return nil, err
	}
	if !r.Exists() {
		return nil, ErrNoHistory
	}
	if _, ok := r.head(); !ok {
		return nil, nil
	}
	cur, start := path, "HEAD"
	var entries []Entry
	for len(entries) < limit {
		want := limit - len(entries)
		args := []string{"log", "--format=%H%x00%h%x00%ct%x00%(trailers:key=" + sessionTrailer + ",valueonly,separator=%x2C)", "-n", strconv.Itoa(want), start}
		if cur != "" {
			args = append(args, "--", cur)
		}
		out, err := r.run(nil, nil, args...)
		if err != nil {
			return nil, err
		}
		lines := strings.FieldsFunc(string(out), func(c rune) bool { return c == '\n' })
		var oldest string
		var oldestChanges []Change
		for _, line := range lines {
			f := strings.Split(line, "\x00")
			if len(f) != 4 {
				continue
			}
			changes, err := r.changes(f[0])
			if err != nil {
				return nil, err
			}
			sec, _ := strconv.ParseInt(f[2], 10, 64)
			e := Entry{Hash: f[1], Time: time.Unix(sec, 0), Changes: under(changes, cur)}
			if f[3] != "" {
				e.Sessions = strings.Split(f[3], ",")
			}
			entries = append(entries, e)
			oldest, oldestChanges = f[0], changes
		}
		if cur == "" || len(lines) == want || oldest == "" {
			break
		}
		old := movedFrom(oldestChanges, cur)
		if old == "" {
			break
		}
		out, err = r.run(nil, nil, "rev-parse", "--quiet", "--verify", oldest+"^")
		if err != nil {
			break
		}
		cur, start = old, strings.TrimSpace(string(out))
	}
	return entries, nil
}

func (r *Repo) changes(hash string) ([]Change, error) {
	out, err := r.run(nil, nil, "diff-tree", "--no-commit-id", "-r", "-M", "--name-status", "-z", "--root", hash)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func under(changes []Change, path string) []Change {
	if path == "" {
		return changes
	}
	var kept []Change
	for _, c := range changes {
		if within(c.Path, path) || (c.OldPath != "" && within(c.OldPath, path)) {
			kept = append(kept, c)
		}
	}
	return kept
}

// The path that was renamed to path in changes, or the folder whose files were renamed into it; "" when nothing was.
func movedFrom(changes []Change, path string) string {
	for _, c := range changes {
		if c.Status != "R" {
			continue
		}
		if c.Path == path {
			return c.OldPath
		}
		if strings.HasPrefix(c.Path, path+"/") {
			rest := c.Path[len(path):]
			if strings.HasSuffix(c.OldPath, rest) && len(c.OldPath) > len(rest) {
				return strings.TrimSuffix(c.OldPath, rest)
			}
		}
	}
	return ""
}

var revPattern = regexp.MustCompile(`^[0-9a-f]{4,40}$`)

// Show returns a file's content at rev, a commit id as Log prints it.
func (r *Repo) Show(rev, path string) ([]byte, error) {
	if _, err := GitPath(); err != nil {
		return nil, err
	}
	if !r.Exists() {
		return nil, ErrNoHistory
	}
	if !revPattern.MatchString(rev) {
		return nil, fmt.Errorf("bibliohistory: %q is not a commit id", rev)
	}
	if path == "" {
		return nil, fmt.Errorf("bibliohistory: show needs a file path")
	}
	out, err := r.run(nil, nil, "cat-file", "-t", rev+":"+path)
	if err != nil {
		return nil, fmt.Errorf("bibliohistory: %s does not exist at %s", path, rev)
	}
	if kind := strings.TrimSpace(string(out)); kind != "blob" {
		return nil, fmt.Errorf("bibliohistory: %s is a folder at %s, show takes a file", path, rev)
	}
	return r.run(nil, nil, "cat-file", "blob", rev+":"+path)
}

// RelPath turns a path given on the command line into one relative to the project's biblio folder. It accepts a path
// already relative to biblio, one starting with biblio/, or an absolute path inside the project's biblio.
func RelPath(projectPath, arg string) (string, error) {
	if arg == "" {
		return "", nil
	}
	p := arg
	if filepath.IsAbs(p) {
		var rel string
		var err error
		for _, base := range biblioRoots(projectPath) {
			if rel, err = filepath.Rel(base, filepath.Clean(p)); err == nil && !escapes(rel) {
				break
			}
		}
		if err != nil || escapes(rel) {
			return "", fmt.Errorf("bibliohistory: %s is not inside this project's biblio", arg)
		}
		p = rel
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "biblio" || p == "." {
		return "", nil
	}
	p = strings.TrimPrefix(p, "biblio/")
	if escapes(p) {
		return "", fmt.Errorf("bibliohistory: %s is not inside this project's biblio", arg)
	}
	return p, nil
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/")
}

func biblioRoots(projectPath string) []string {
	biblio := filepath.Join(projectPath, "biblio")
	roots := []string{biblio}
	if resolved, err := filepath.EvalSymlinks(biblio); err == nil && resolved != biblio {
		roots = append(roots, resolved)
	}
	return roots
}
