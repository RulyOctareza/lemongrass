package guard

import (
	"os"
	"path/filepath"
)

const claudeTempDirEnv = "CLAUDE_CODE_TMPDIR"

// TempRoots are the directories a reboot can wipe: the OS temp dir, /tmp, and Claude Code's own temp root when it is moved.
func TempRoots() []string {
	var roots []string
	for _, dir := range []string{os.TempDir(), "/tmp", os.Getenv(claudeTempDirEnv)} {
		if dir == "" {
			continue
		}
		if resolved := resolveFully(dir); !containsArg(roots, resolved) {
			roots = append(roots, resolved)
		}
	}
	return roots
}

// TempWrite returns the first path a file tool writes inside one of roots, or "" when it writes none there. Shell commands are not judged, since what they write (test builds, logs, probes) can be regenerated, while a file the model authors, such as a PoC, is expensive to lose to a reboot. A path inside in.ProjectRoot never counts, so a project that itself lives in a temp dir can still be worked on.
func TempWrite(in Input, roots []string) string {
	if len(roots) == 0 || in.Command != "" || !in.Writes {
		return ""
	}
	project := ""
	if in.ProjectRoot != "" {
		project = resolveFully(in.ProjectRoot)
	}
	for _, p := range in.Paths {
		if landsInTemp(ResolvePath(p, in.Cwd, in.Home), roots, project) {
			return p
		}
	}
	return ""
}

func landsInTemp(p string, roots []string, project string) bool {
	location := filepath.Join(resolveExisting(filepath.Dir(p)), filepath.Base(p))
	for _, candidate := range []string{location, resolveFully(p)} {
		if project != "" && withinOrEqual(candidate, project) {
			continue
		}
		for _, root := range roots {
			if withinOrEqual(candidate, root) {
				return true
			}
		}
	}
	return false
}
