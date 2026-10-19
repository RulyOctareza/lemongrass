package node

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/faizalv/lemongrass/internal/project"
)

// The knowledge folder every project keeps at its root, as internal/hook names it.
const biblioDir = "biblio"

// These index every law and every handover, so serving one would reveal what the grant leaves out.
const (
	lawsIndex     = "summary.md"
	handoverIndex = "whiteboard.md"
)

// ReadCap is the most bytes of one file a single read returns.
const ReadCap = 16 * 1024

var (
	ErrNotFound  = errors.New("node: not found")
	ErrBadOffset = errors.New("node: offset is past the end of the file")
)

// Library matches every requested name against directory listings it reads itself, never joining a name into a path as given.
type Library struct {
	Projects func() ([]project.Project, error)
}

func NewLibrary() *Library {
	return &Library{Projects: project.List}
}

// Read answers the contents for an empty path, then alias, alias/books, alias/tasks, alias/laws, alias/handovers, a book or task, and a file read from offset.
func (l *Library) Read(cfg Config, g Grant, path string, offset int) (string, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return l.contents(cfg, g, "")
	}
	parts := strings.Split(path, "/")
	alias := parts[0]
	if _, ok := shareFor(g, alias); !ok {
		return "", ErrNotFound
	}
	if len(parts) == 1 {
		return l.contents(cfg, g, alias)
	}
	root, err := l.projectRoot(cfg, alias)
	if err != nil {
		return "", err
	}
	share, _ := shareFor(g, alias)
	switch parts[1] {
	case "books":
		return readBooks(root, alias, share, parts[2:], offset)
	case "tasks":
		return readTasks(root, alias, share, parts[2:], offset)
	case "laws":
		return readDocs(filepath.Join(root, "laws"), lawsIndex, alias+"/laws/", share.Laws, parts[2:], offset)
	case "handovers":
		return readDocs(filepath.Join(root, "handover"), handoverIndex, alias+"/handovers/", share.Handovers, parts[2:], offset)
	}
	return "", ErrNotFound
}

func (l *Library) contents(cfg Config, g Grant, only string) (string, error) {
	var lines []string
	for _, share := range mergedShares(g) {
		if only != "" && share.Alias != only {
			continue
		}
		root, err := l.projectRoot(cfg, share.Alias)
		if err != nil {
			continue
		}
		books, _ := grantedBooks(root, share)
		for _, b := range books {
			lines = append(lines, share.Alias+"/books/"+b)
		}
		for _, slug := range share.Tasks {
			if _, archived, err := taskDir(root, slug); err == nil {
				line := share.Alias + "/tasks/" + slug
				if archived {
					line += " (archived)"
				}
				lines = append(lines, line)
			}
		}
		for _, name := range grantedDocs(filepath.Join(root, "laws"), lawsIndex, share.Laws) {
			lines = append(lines, share.Alias+"/laws/"+name)
		}
		for _, name := range grantedDocs(filepath.Join(root, "handover"), handoverIndex, share.Handovers) {
			lines = append(lines, share.Alias+"/handovers/"+name)
		}
	}
	if len(lines) == 0 {
		return "Nothing is shared with this token.\n", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func (l *Library) projectRoot(cfg Config, alias string) (string, error) {
	id := ""
	for _, p := range cfg.Projects {
		if p.Alias == alias {
			id = p.ProjectID
			break
		}
	}
	if id == "" {
		return "", ErrNotFound
	}
	projects, err := l.Projects()
	if err != nil {
		return "", fmt.Errorf("node: listing projects: %w", err)
	}
	for _, p := range projects {
		if p.ID == id {
			return filepath.Join(p.Path, biblioDir), nil
		}
	}
	return "", ErrNotFound
}

// A grant can name the same alias in more than one share, and reads see their union.
func mergedShares(g Grant) []Share {
	var out []Share
	for _, s := range g.Shares {
		if i := slices.IndexFunc(out, func(o Share) bool { return o.Alias == s.Alias }); i >= 0 {
			out[i].Books = append(out[i].Books, s.Books...)
			out[i].Tasks = append(out[i].Tasks, s.Tasks...)
			out[i].Laws = append(out[i].Laws, s.Laws...)
			out[i].Handovers = append(out[i].Handovers, s.Handovers...)
			continue
		}
		out = append(out, Share{
			Alias:     s.Alias,
			Books:     slices.Clone(s.Books),
			Tasks:     slices.Clone(s.Tasks),
			Laws:      slices.Clone(s.Laws),
			Handovers: slices.Clone(s.Handovers),
		})
	}
	return out
}

func shareFor(g Grant, alias string) (Share, bool) {
	for _, s := range mergedShares(g) {
		if s.Alias == alias {
			return s, true
		}
	}
	return Share{}, false
}

// BookIdentity is a book folder's title and creation date, the part a tag change does not alter.
func BookIdentity(name string) string {
	open := strings.Index(name, "[")
	end := strings.Index(name, "]")
	if open <= 0 || end < open {
		return ""
	}
	return name[:end+1]
}

func readBooks(root, alias string, share Share, rest []string, offset int) (string, error) {
	switch len(rest) {
	case 0:
		books, err := grantedBooks(root, share)
		if err != nil {
			return "", err
		}
		return listing(alias+"/books/", books), nil
	case 1, 2:
		dir, err := bookDir(root, share, rest[0])
		if err != nil {
			return "", err
		}
		if len(rest) == 1 {
			files, err := markdownFiles(dir)
			if err != nil {
				return "", err
			}
			return listing("", files), nil
		}
		return readFile(dir, rest[1], offset)
	}
	return "", ErrNotFound
}

func readTasks(root, alias string, share Share, rest []string, offset int) (string, error) {
	switch len(rest) {
	case 0:
		var slugs []string
		for _, slug := range share.Tasks {
			if _, _, err := taskDir(root, slug); err == nil {
				slugs = append(slugs, slug)
			}
		}
		return listing(alias+"/tasks/", slugs), nil
	case 1, 2:
		if !slices.Contains(share.Tasks, rest[0]) {
			return "", ErrNotFound
		}
		dir, archived, err := taskDir(root, rest[0])
		if err != nil {
			return "", err
		}
		if len(rest) == 2 {
			return readFile(dir, rest[1], offset)
		}
		files, err := markdownFiles(dir)
		if err != nil {
			return "", err
		}
		out := listing("", files)
		if archived {
			out = "(archived task)\n" + out
		}
		return out, nil
	}
	return "", ErrNotFound
}

// readDocs serves single-file documents, laws or handovers, listing the granted ones or reading one.
func readDocs(dir, index, prefix string, granted, rest []string, offset int) (string, error) {
	if len(granted) == 0 {
		return "", ErrNotFound
	}
	switch len(rest) {
	case 0:
		return listing(prefix, grantedDocs(dir, index, granted)), nil
	case 1:
		path, _, err := docFile(dir, index, granted, rest[0])
		if err != nil {
			return "", err
		}
		return readFile(filepath.Dir(path), filepath.Base(path), offset)
	}
	return "", ErrNotFound
}

// grantedDocs lists the granted documents that exist, with " (archived)" on those found in the archive.
func grantedDocs(dir, index string, granted []string) []string {
	var out []string
	for _, name := range granted {
		if _, archived, err := docFile(dir, index, granted, name); err == nil {
			if archived {
				name += " (archived)"
			}
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// docFile finds a granted document in dir first and then in dir/archive, matching against listings only.
func docFile(dir, index string, granted []string, requested string) (path string, archived bool, err error) {
	name := strings.TrimSuffix(requested, ".md")
	if name+".md" == index || !slices.Contains(granted, name) {
		return "", false, ErrNotFound
	}
	for _, d := range []struct {
		dir      string
		archived bool
	}{{dir, false}, {filepath.Join(dir, "archive"), true}} {
		files, err := markdownFiles(d.dir)
		if err == nil && slices.Contains(files, name+".md") {
			return filepath.Join(d.dir, name+".md"), d.archived, nil
		}
	}
	return "", false, ErrNotFound
}

func listing(prefix string, names []string) string {
	if len(names) == 0 {
		return "(empty)\n"
	}
	var b strings.Builder
	for _, n := range names {
		b.WriteString(prefix + n + "\n")
	}
	return b.String()
}

// grantedBooks lists the folder names of granted books whose identity resolves to exactly one folder.
func grantedBooks(root string, share Share) ([]string, error) {
	dirs, err := subdirs(filepath.Join(root, "books"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range share.Books {
		if name, ok := uniqueBook(dirs, id); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func bookDir(root string, share Share, requested string) (string, error) {
	id := BookIdentity(requested)
	if id == "" || !slices.Contains(share.Books, id) {
		return "", ErrNotFound
	}
	booksDir := filepath.Join(root, "books")
	dirs, err := subdirs(booksDir)
	if err != nil {
		return "", err
	}
	name, ok := uniqueBook(dirs, id)
	if !ok {
		return "", ErrNotFound
	}
	return filepath.Join(booksDir, name), nil
}

// Two folders with one identity break the convention, so neither is served.
func uniqueBook(dirs []string, id string) (string, bool) {
	found := ""
	for _, d := range dirs {
		if BookIdentity(d) == id {
			if found != "" {
				return "", false
			}
			found = d
		}
	}
	return found, found != ""
}

// taskDir finds a task among active tasks first, then archived ones.
func taskDir(root, slug string) (dir string, archived bool, err error) {
	scratch := filepath.Join(root, "scratchpad")
	active, err := subdirs(scratch)
	if err != nil {
		return "", false, err
	}
	if slug != "archive" && slices.Contains(active, slug) {
		return filepath.Join(scratch, slug), false, nil
	}
	archiveDir := filepath.Join(scratch, "archive")
	closed, err := subdirs(archiveDir)
	if err != nil {
		return "", false, err
	}
	if slices.Contains(closed, slug) {
		return filepath.Join(archiveDir, slug), true, nil
	}
	return "", false, ErrNotFound
}

// subdirs lists real directories only, so a symlinked entry can never be followed.
func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("node: listing %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.Type()&os.ModeType == os.ModeDir {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// markdownFiles lists regular .md files only, skipping symlinks and subdirectories.
func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNotFound
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func readFile(dir, requested string, offset int) (string, error) {
	files, err := markdownFiles(dir)
	if err != nil {
		return "", err
	}
	name := ""
	for _, f := range files {
		if f == requested || f == requested+".md" {
			name = f
			break
		}
	}
	if name == "" {
		return "", ErrNotFound
	}
	full := filepath.Join(dir, name)
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrNotFound
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("node: reading %s: %w", name, err)
	}
	return cut(data, offset)
}

// cut returns at most ReadCap bytes from offset, ending at a line break when one is in reach and naming the next offset when cut.
func cut(data []byte, offset int) (string, error) {
	if offset < 0 || offset > len(data) {
		return "", ErrBadOffset
	}
	end := offset + ReadCap
	if end >= len(data) {
		return string(data[offset:]), nil
	}
	if nl := strings.LastIndexByte(string(data[offset:end]), '\n'); nl > 0 {
		end = offset + nl + 1
	} else {
		for end > offset && data[end]&0xC0 == 0x80 {
			end--
		}
	}
	return string(data[offset:end]) + fmt.Sprintf("\n[cut: %d of %d bytes shown, read again with offset %d]\n", end, len(data), end), nil
}
