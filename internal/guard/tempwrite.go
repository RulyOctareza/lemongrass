package guard

import (
	"os"
	"path/filepath"
	"strings"
)

const claudeTempDirEnv = "CLAUDE_CODE_TMPDIR"

// Flags of the write commands below that take a value, so the value is not read as a target.
var tempWriteValueFlags = map[string]map[string]bool{
	"cp":      flagSet("-S", "--suffix"),
	"mv":      flagSet("-S", "--suffix"),
	"install": flagSet("-m", "--mode", "-o", "--owner", "-g", "--group", "-S", "--suffix"),
	"mkdir":   flagSet("-m", "--mode"),
	"touch":   flagSet("-d", "--date", "-t", "-r", "--reference"),
	"tee":     flagSet(),
}

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

// TempWrite returns the first target a call writes inside one of roots, or "" when it writes none there. A target inside in.ProjectRoot never counts, so a project that itself lives in a temp dir can still be worked on. Like the rest of the guard it is a tripwire: a target built from a variable other than $TMPDIR or $HOME is not judged.
func TempWrite(in Input, roots []string) string {
	if len(roots) == 0 {
		return ""
	}
	t := tempCheck{in: in, roots: roots}
	if in.ProjectRoot != "" {
		t.project = resolveFully(in.ProjectRoot)
	}
	if in.Command == "" {
		if !in.Writes {
			return ""
		}
		for _, p := range in.Paths {
			if t.lands(p, []string{in.Cwd}) {
				return p
			}
		}
		return ""
	}
	script, err := parseShell(in.Command)
	if err != nil {
		return ""
	}
	dirs := t.shellDirs(script)
	for _, redirect := range script.Redirects {
		if redirect.Write && t.lands(redirect.Path, dirs) {
			return redirect.Path
		}
	}
	for _, call := range script.Calls {
		for _, target := range writeTargets(call) {
			if t.lands(target, dirs) {
				return target
			}
		}
	}
	return ""
}

type tempCheck struct {
	in      Input
	roots   []string
	project string
}

// shellDirs is the starting cwd plus every literal cd or pushd target resolved against each directory before it, so a relative write after a cd into a temp dir is caught.
func (t tempCheck) shellDirs(script shellScript) []string {
	dirs := []string{t.in.Cwd}
	for _, call := range script.Calls {
		if !directoryChangers[call.Name] {
			continue
		}
		target, ok := directoryTarget(call)
		if !ok {
			continue
		}
		target, ok = t.expand(target)
		if !ok {
			continue
		}
		for _, dir := range dirs {
			if next := resolveFully(ResolvePath(target, dir, t.in.Home)); !containsArg(dirs, next) {
				dirs = append(dirs, next)
			}
		}
	}
	return dirs
}

func (t tempCheck) lands(target string, dirs []string) bool {
	target, ok := t.expand(target)
	if !ok || target == "" {
		return false
	}
	for _, dir := range dirs {
		p := ResolvePath(target, dir, t.in.Home)
		location := filepath.Join(resolveExisting(filepath.Dir(p)), filepath.Base(p))
		for _, candidate := range []string{location, resolveFully(p)} {
			if t.project != "" && withinOrEqual(candidate, t.project) {
				continue
			}
			for _, root := range t.roots {
				if withinOrEqual(candidate, root) {
					return true
				}
			}
		}
	}
	return false
}

// expand turns a leading $TMPDIR into the OS temp dir and reports false for any other target the guard cannot place.
func (t tempCheck) expand(target string) (string, bool) {
	for _, prefix := range []string{"$TMPDIR", "${TMPDIR}"} {
		if strings.HasPrefix(target, prefix) {
			return os.TempDir() + strings.TrimPrefix(target, prefix), true
		}
	}
	return target, literalTarget(target)
}

// writeTargets lists the paths a call creates or overwrites. A bare mktemp template lands in $TMPDIR.
func writeTargets(call shellCall) []string {
	if call.Name == "mktemp" {
		return mktempTargets(call.Args)
	}
	valueFlags, ok := tempWriteValueFlags[call.Name]
	if !ok {
		return nil
	}
	var operands []string
	targetDir := ""
	directoryMode := false
	flagsDone := false
	for i := 0; i < len(call.Args); i++ {
		arg := call.Args[i]
		switch {
		case flagsDone || arg == "-" || !strings.HasPrefix(arg, "-"):
			operands = append(operands, arg)
		case arg == "--":
			flagsDone = true
		case arg == "-t" || arg == "--target-directory":
			if i+1 < len(call.Args) {
				i++
				targetDir = call.Args[i]
			}
		case strings.HasPrefix(arg, "--target-directory="):
			targetDir = strings.TrimPrefix(arg, "--target-directory=")
		case call.Name == "install" && (arg == "-d" || arg == "--directory"):
			directoryMode = true
		case valueFlags[arg]:
			i++
		}
	}
	if targetDir != "" {
		return []string{targetDir}
	}
	switch call.Name {
	case "cp", "mv":
		if len(operands) < 2 {
			return nil
		}
		return operands[len(operands)-1:]
	case "install":
		if directoryMode || len(operands) < 2 {
			return operands
		}
		return operands[len(operands)-1:]
	}
	return operands
}

func mktempTargets(args []string) []string {
	dir, template, useTempDir := "", "", false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-p":
			if i+1 < len(args) {
				i++
				dir = args[i]
			}
		case strings.HasPrefix(arg, "--tmpdir="):
			dir = strings.TrimPrefix(arg, "--tmpdir=")
		case arg == "--tmpdir" || arg == "-t":
			useTempDir = true
		case strings.HasPrefix(arg, "-"):
		default:
			template = arg
		}
	}
	switch {
	case dir != "":
		return []string{filepath.Join(dir, template)}
	case template == "" || useTempDir || !strings.Contains(template, "/"):
		return []string{"$TMPDIR/" + template}
	}
	return []string{template}
}
