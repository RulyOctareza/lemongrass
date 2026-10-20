package bibliohistory

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

type fakeSystem struct {
	goos  string
	path  string // what PATH resolves git to, "" for nothing
	files map[string]bool
	env   map[string]string
	clt   bool
}

func (f fakeSystem) lookup() gitLookup {
	return gitLookup{
		goos: f.goos,
		lookPath: func(string) (string, error) {
			if f.path == "" {
				return "", exec.ErrNotFound
			}
			return f.path, nil
		},
		isFile:       func(p string) bool { return f.files[p] },
		getenv:       func(k string) string { return f.env[k] },
		cltInstalled: func() bool { return f.clt },
	}
}

func TestFindGit(t *testing.T) {
	winGit := filepath.Join(`C:\Program Files`, "Git", "cmd", "git.exe")
	userGit := filepath.Join(`C:\Users\a\AppData\Local`, "Programs", "Git", "cmd", "git.exe")
	cases := []struct {
		name string
		sys  fakeSystem
		want string
	}{
		{"linux on PATH", fakeSystem{goos: "linux", path: "/usr/bin/git"}, "/usr/bin/git"},
		{"linux minimal PATH", fakeSystem{goos: "linux", files: map[string]bool{"/usr/local/bin/git": true}}, "/usr/local/bin/git"},
		{"linux none", fakeSystem{goos: "linux"}, ""},
		{"mac stub without tools", fakeSystem{goos: "darwin", path: macGitStub, files: map[string]bool{macGitStub: true}}, ""},
		{"mac stub with tools", fakeSystem{goos: "darwin", path: macGitStub, files: map[string]bool{macGitStub: true}, clt: true}, macGitStub},
		{"mac homebrew from Finder", fakeSystem{goos: "darwin", files: map[string]bool{"/opt/homebrew/bin/git": true, macGitStub: true}}, "/opt/homebrew/bin/git"},
		{"mac stub on PATH, homebrew present", fakeSystem{goos: "darwin", path: macGitStub, files: map[string]bool{"/usr/local/bin/git": true}}, "/usr/local/bin/git"},
		{"windows on PATH", fakeSystem{goos: "windows", path: `C:\git\git.exe`}, `C:\git\git.exe`},
		{"windows program files", fakeSystem{goos: "windows", env: map[string]string{"ProgramFiles": `C:\Program Files`}, files: map[string]bool{winGit: true}}, winGit},
		{"windows user install", fakeSystem{goos: "windows", env: map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`}, files: map[string]bool{userGit: true}}, userGit},
		{"windows none", fakeSystem{goos: "windows", env: map[string]string{"ProgramFiles": `C:\Program Files`}}, ""},
	}
	for _, c := range cases {
		got, err := findGit(c.sys.lookup())
		if c.want == "" {
			if !errors.Is(err, ErrNoGit) {
				t.Errorf("%s: got %q, %v; want ErrNoGit", c.name, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestMissingGit(t *testing.T) {
	r, _ := newTestRepo(t)
	gitMu.Lock()
	gitFound = ""
	gitMu.Unlock()
	old := systemGit
	systemGit = fakeSystem{goos: "linux"}.lookup
	defer func() { systemGit = old }()

	if _, err := r.Log("", 5); !errors.Is(err, ErrNoGit) {
		t.Fatalf("log err = %v", err)
	}
	if _, err := r.Snapshot(nil); !errors.Is(err, ErrNoGit) {
		t.Fatalf("snapshot err = %v", err)
	}
}
