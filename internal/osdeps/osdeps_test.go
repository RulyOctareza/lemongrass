// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const fedoraRelease = "NAME=\"Fedora Linux\"\nID=fedora\nVERSION_ID=43\n"

type fakeOut struct {
	out string
	err error
}

var errExit = errors.New("exit status 1")

// fakeMachine stands in for a whole machine: files with their stats, what PATH finds, and each command's output.
type fakeMachine struct {
	goos      string
	osRelease string
	files     map[string]FileStat
	onPath    map[string]string
	links     map[string]string
	env       map[string]string
	commands  map[string]fakeOut

	mu    sync.Mutex
	calls []string
}

func newFedora() *fakeMachine {
	return &fakeMachine{
		goos:      "linux",
		osRelease: fedoraRelease,
		files:     map[string]FileStat{},
		onPath:    map[string]string{},
		links:     map[string]string{},
		commands:  map[string]fakeOut{},
	}
}

// install puts a binary on PATH with its version output and its rpm owner; owner "" means installed by hand.
func (m *fakeMachine) install(name, path, versionOut, owner string) {
	m.files[path] = FileStat{Size: 100, ModTime: 1, Exec: true}
	m.onPath[name] = path
	d, _ := Find(name)
	m.commands[path+" "+strings.Join(d.VersionArgs, " ")] = fakeOut{out: versionOut}
	if owner == "" {
		m.commands["rpm -qf --qf %{NAME}\n "+path] = fakeOut{out: "file " + path + " is not owned by any package\n", err: errExit}
	} else {
		m.commands["rpm -qf --qf %{NAME}\n "+path] = fakeOut{out: owner + "\n"}
	}
}

func (m *fakeMachine) system() System {
	return System{
		GOOS: m.goos,
		Home: "/home/u",
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			m.mu.Lock()
			m.calls = append(m.calls, key)
			m.mu.Unlock()
			if r, ok := m.commands[key]; ok {
				return []byte(r.out), r.err
			}
			return nil, errors.New("exec: not found")
		},
		LookPath: func(name string) (string, error) {
			if p, ok := m.onPath[name]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		Stat: func(path string) (FileStat, bool) {
			st, ok := m.files[path]
			return st, ok
		},
		EvalSymlinks: func(path string) (string, error) {
			if real, ok := m.links[path]; ok {
				return real, nil
			}
			return path, nil
		},
		ReadFile: func(path string) ([]byte, error) {
			if path == "/etc/os-release" && m.osRelease != "" {
				return []byte(m.osRelease), nil
			}
			return nil, errors.New("no such file")
		},
		Getenv: func(k string) string { return m.env[k] },
	}
}

func (m *fakeMachine) callCount(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func check(t *testing.T, m *fakeMachine) map[string]DepStatus {
	t.Helper()
	report, err := Check(context.Background(), m.system(), filepath.Join(t.TempDir(), "deps.json"), false)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	byName := map[string]DepStatus{}
	for _, d := range report.Deps {
		byName[d.Name] = d
	}
	return byName
}

func TestParseVersion(t *testing.T) {
	cases := []struct{ name, out, want string }{
		{"git", "git version 2.56.0\n", "2.56.0"},
		{"git", "git version 2.39.5 (Apple Git-154)\n", "2.39.5"},
		{"git", "git version 2.45.1.windows.1\n", "2.45.1"},
		{"adb", "Android Debug Bridge version 1.0.41\nVersion 37.0.0p1-12147458\nInstalled as /usr/bin/adb\nRunning on Linux\n", "37.0.0"},
		{"adb", "Android Debug Bridge version 1.0.41\n", "1.0.41"},
		{"scrcpy", "scrcpy 3.1 <https://github.com/Genymobile/scrcpy>\n\nDependencies (compiled / linked):\n - SDL: 2.30.0 / 2.30.0\n", "3.1"},
		{"ffmpeg", "ffmpeg version 7.1.5 Copyright (c) 2000-2026 the FFmpeg developers\n", "7.1.5"},
		{"ffmpeg", "ffmpeg version n7.1-12-gabcdef Copyright\n", "7.1"},
		{"ffmpeg", "ffmpeg version N-12345-gabcdef Copyright\n", ""},
		{"ffmpeg", "ffmpeg version 5.1.6-0+deb12u1 Copyright\n", "5.1.6"},
		{"ssh", "OpenSSH_10.0p2, OpenSSL 3.5.1 1 Jul 2025\n", "10.0p2"},
		{"git", "bash: git: command not found\n", ""},
	}
	for _, c := range cases {
		if got := ParseVersion(c.name, c.out); got != c.want {
			t.Errorf("ParseVersion(%s, %q) = %q, want %q", c.name, c.out, got, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.56.0", "2.45.0", 1},
		{"2.44.9", "2.45.0", -1},
		{"2.45", "2.45.0", 0},
		{"2.45.1", "2.45.0", 1},
		{"10.0p2", "10.0", 0},
		{"2.100.0", "2.45.0", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	cases := []struct{ data, family, id, version string }{
		{fedoraRelease, FamilyFedora, "fedora", "43"},
		{"ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.6\"\n", FamilyFedora, "rocky", "9.6"},
		{"ID=\"rhel\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"10.2\"\n", FamilyFedora, "rhel", "10.2"},
		{"ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\n", FamilyDebian, "ubuntu", "24.04"},
		{"# comment\nID=linuxmint\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"22\"\n", FamilyDebian, "linuxmint", "22"},
		{"ID=arch\n", FamilyUnknown, "arch", ""},
	}
	for _, c := range cases {
		got := ParseOSRelease(c.data)
		if got.Family != c.family || got.ID != c.id || got.Version != c.version {
			t.Errorf("ParseOSRelease(%q) = %+v", c.data, got)
		}
	}
}

func TestCheckFedora(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "git version 2.56.0\n", "git-core")
	m.install("ffmpeg", "/usr/bin/ffmpeg", "ffmpeg version 7.1.5 Copyright\n", "ffmpeg")
	m.install("ssh", "/usr/bin/ssh", "OpenSSH_10.0p2, OpenSSL 3.5.1\n", "openssh-clients")
	got := check(t, m)

	git := got["git"]
	if git.Status != StatusOK || git.Version != "2.56.0" || git.Origin != (Origin{OriginPackage, "git-core"}) || git.Offer != nil {
		t.Errorf("git = %+v", git)
	}
	adb := got["adb"]
	if adb.Status != StatusMissing || adb.Origin.Kind != OriginNone || adb.Offer == nil ||
		*adb.Offer != (Offer{ActionInstall, "android-tools", RouteHelper}) {
		t.Errorf("adb = %+v", adb)
	}
	scrcpy := got["scrcpy"]
	if scrcpy.Status != StatusMissing || scrcpy.Offer != nil || !strings.Contains(scrcpy.Message, "zeno/scrcpy") {
		t.Errorf("scrcpy = %+v", scrcpy)
	}
	ffmpeg := got["ffmpeg"]
	if ffmpeg.Status != StatusOK || ffmpeg.Origin.Package != "ffmpeg" || ffmpeg.Offer != nil {
		t.Errorf("ffmpeg = %+v", ffmpeg)
	}
	if ssh := got["ssh"]; ssh.Status != StatusOK || ssh.Version != "10.0p2" || ssh.Offer != nil {
		t.Errorf("ssh = %+v", ssh)
	}
}

func TestCheckFreshInstallUsesFfmpegFree(t *testing.T) {
	m := newFedora()
	if ffmpeg := check(t, m)["ffmpeg"]; ffmpeg.Offer == nil || ffmpeg.Offer.Package != "ffmpeg-free" {
		t.Errorf("ffmpeg = %+v", ffmpeg)
	}
}

func TestCheckGitBelowFloorFromPackage(t *testing.T) {
	m := newFedora()
	m.osRelease = "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\n"
	m.install("git", "/usr/bin/git", "git version 2.43.5\n", "git-core")
	git := check(t, m)["git"]
	if git.Status != StatusBelowFloor || git.Offer == nil || *git.Offer != (Offer{ActionUpgrade, "git-core", RouteHelper}) {
		t.Errorf("git = %+v", git)
	}
}

func TestCheckGitBelowFloorManual(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/local/bin/git", "git version 2.40.1\n", "")
	git := check(t, m)["git"]
	if git.Status != StatusBelowFloor || git.Origin.Kind != OriginManual || git.Offer != nil ||
		!strings.Contains(git.Message, "/usr/local/bin/git was installed by hand") {
		t.Errorf("git = %+v", git)
	}
}

func TestCheckGitUnknownVersion(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "something else\n", "git-core")
	git := check(t, m)["git"]
	if git.Status != StatusUnknownVersion || git.Offer != nil || git.Message == "" {
		t.Errorf("git = %+v", git)
	}
}

func TestCheckOriginThroughSymlink(t *testing.T) {
	m := newFedora()
	m.install("ffmpeg", "/usr/bin/ffmpeg", "ffmpeg version 7.1.5\n", "")
	m.links["/usr/bin/ffmpeg"] = "/usr/lib/ffmpeg/ffmpeg"
	m.files["/usr/lib/ffmpeg/ffmpeg"] = FileStat{Size: 1, Exec: true}
	m.commands["rpm -qf --qf %{NAME}\n /usr/lib/ffmpeg/ffmpeg"] = fakeOut{out: "ffmpeg-free\nffmpeg-free\n"}
	if ffmpeg := check(t, m)["ffmpeg"]; ffmpeg.Origin != (Origin{OriginPackage, "ffmpeg-free"}) {
		t.Errorf("ffmpeg = %+v", ffmpeg)
	}
}

func TestCheckRpmFailureIsUnknown(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "git version 2.56.0\n", "git-core")
	m.commands["rpm -qf --qf %{NAME}\n /usr/bin/git"] = fakeOut{out: "error: rpmdb: lock held\n", err: errExit}
	if git := check(t, m)["git"]; git.Origin.Kind != OriginUnknown {
		t.Errorf("git = %+v", git)
	}
}

func TestCheckAdbInAndroidSdk(t *testing.T) {
	m := newFedora()
	adb := "/home/u/Android/Sdk/platform-tools/adb"
	m.install("adb", adb, "Android Debug Bridge version 1.0.41\nVersion 35.0.2-12147458\n", "")
	delete(m.onPath, "adb")
	got := check(t, m)["adb"]
	if got.Status != StatusOK || got.Path != adb || got.Version != "35.0.2" || got.Origin.Kind != OriginManual || got.Offer != nil {
		t.Errorf("adb = %+v", got)
	}
}

func TestCheckFfmpegNightlyHasNoVersion(t *testing.T) {
	m := newFedora()
	m.install("ffmpeg", "/usr/local/bin/ffmpeg", "ffmpeg version N-12345-gabcdef\n", "")
	if ffmpeg := check(t, m)["ffmpeg"]; ffmpeg.Status != StatusOK || ffmpeg.Version != "" {
		t.Errorf("ffmpeg = %+v", ffmpeg)
	}
}

func TestCheckSshMissingIsNeverOffered(t *testing.T) {
	m := newFedora()
	ssh := check(t, m)["ssh"]
	if ssh.Status != StatusMissing || ssh.Offer != nil || !strings.Contains(ssh.Message, "openssh-clients") {
		t.Errorf("ssh = %+v", ssh)
	}
}

func TestCheckAtomicDesktop(t *testing.T) {
	m := newFedora()
	m.files["/run/ostree-booted"] = FileStat{}
	m.install("git", "/usr/bin/git", "git version 2.44.0\n", "git-core")
	got := check(t, m)
	if adb := got["adb"]; adb.Offer != nil || !strings.Contains(adb.Message, "rpm-ostree install android-tools") {
		t.Errorf("adb = %+v", adb)
	}
	if git := got["git"]; git.Status != StatusBelowFloor || git.Offer != nil || !strings.Contains(git.Message, "image") {
		t.Errorf("git = %+v", git)
	}
}

func TestCheckFamilyNotBuiltYet(t *testing.T) {
	m := newFedora()
	m.osRelease = "ID=debian\nVERSION_ID=\"12\"\n"
	m.install("git", "/usr/bin/git", "git version 2.39.5\n", "git")
	got := check(t, m)
	if git := got["git"]; git.Status != StatusBelowFloor || git.Origin.Kind != OriginUnknown || git.Offer != nil {
		t.Errorf("git = %+v", git)
	}
	if adb := got["adb"]; adb.Status != StatusMissing || adb.Offer != nil {
		t.Errorf("adb = %+v", adb)
	}
	if m.callCount("rpm ") != 0 {
		t.Errorf("rpm ran on a Debian system")
	}
}

func TestCheckMacGitStubWithoutCommandLineTools(t *testing.T) {
	m := newFedora()
	m.goos = "darwin"
	m.install("git", macGitStub, "git version 2.39.5 (Apple Git-154)\n", "")
	m.commands["sw_vers -productVersion"] = fakeOut{out: "15.1\n"}
	m.commands["xcode-select -p"] = fakeOut{err: errExit}
	report, _ := Check(context.Background(), m.system(), filepath.Join(t.TempDir(), "deps.json"), false)
	if report.OS != (OSInfo{Family: FamilyMacOS, ID: "macos", Version: "15.1"}) {
		t.Errorf("os = %+v", report.OS)
	}
	if report.Deps[0].Status != StatusMissing {
		t.Errorf("git = %+v", report.Deps[0])
	}
	if m.callCount(macGitStub) != 0 {
		t.Errorf("the git stub was run without the Command Line Tools")
	}
}

func TestCheckCache(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "git version 2.56.0\n", "git-core")
	m.install("adb", "/usr/bin/adb", "Android Debug Bridge version 1.0.41\nVersion 37.0.0p1-1\n", "android-tools")
	cache := filepath.Join(t.TempDir(), "deps.json")
	run := func(fresh bool) Report {
		t.Helper()
		report, err := Check(context.Background(), m.system(), cache, fresh)
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		return report
	}

	run(false)
	if m.callCount("/usr/bin/git ") != 1 || m.callCount("rpm ") != 2 {
		t.Fatalf("first run calls = %v", m.calls)
	}
	second := run(false)
	if m.callCount("/usr/bin/git ") != 1 || m.callCount("rpm ") != 2 {
		t.Errorf("an unchanged machine re-ran commands: %v", m.calls)
	}
	if second.Deps[0].Version != "2.56.0" || second.Deps[0].Origin.Package != "git-core" {
		t.Errorf("cached git = %+v", second.Deps[0])
	}

	m.files["/usr/bin/git"] = FileStat{Size: 100, ModTime: 2, Exec: true}
	m.commands["/usr/bin/git --version"] = fakeOut{out: "git version 2.57.0\n"}
	if v := run(false).Deps[0].Version; v != "2.57.0" {
		t.Errorf("after an update git = %s", v)
	}
	if m.callCount("/usr/bin/adb ") != 1 {
		t.Errorf("an unchanged adb was re-run: %v", m.calls)
	}

	run(true)
	if m.callCount("/usr/bin/adb ") != 2 {
		t.Errorf("fresh did not re-run adb: %v", m.calls)
	}

	m.osRelease = "ID=fedora\nVERSION_ID=44\n"
	run(false)
	if m.callCount("/usr/bin/adb ") != 3 {
		t.Errorf("a new OS release reused the cache: %v", m.calls)
	}
}

func TestCheckUnsettledEntryIsNotCached(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "git version 2.56.0\n", "git-core")
	m.commands["rpm -qf --qf %{NAME}\n /usr/bin/git"] = fakeOut{err: errors.New("signal: killed")}
	cache := filepath.Join(t.TempDir(), "deps.json")
	Check(context.Background(), m.system(), cache, false)
	m.install("git", "/usr/bin/git", "git version 2.56.0\n", "git-core")
	report, _ := Check(context.Background(), m.system(), cache, false)
	if report.Deps[0].Origin.Package != "git-core" {
		t.Errorf("an unread origin stuck in the cache: %+v", report.Deps[0])
	}
}

func TestEveryFedoraOfferIsAllowed(t *testing.T) {
	for _, d := range Deps {
		if pkg := d.Packages[FamilyFedora]; pkg != "" && !HelperAllows(FamilyFedora, pkg) {
			t.Errorf("%s installs %s, which lgrassroot refuses", d.Name, pkg)
		}
	}
}

func TestUpgradeOfAnOwnerOutsideTheAllowlistIsNotOffered(t *testing.T) {
	m := newFedora()
	m.install("git", "/usr/bin/git", "git version 2.40.0\n", "git-custom-build")
	git := check(t, m)["git"]
	if git.Status != StatusBelowFloor || git.Offer != nil || git.Message == "" {
		t.Errorf("git = %+v", git)
	}
}

func TestResolveGit(t *testing.T) {
	winGit := filepath.Join(`C:\Program Files`, "Git", "cmd", "git.exe")
	userGit := filepath.Join(`C:\Users\a\AppData\Local`, "Programs", "Git", "cmd", "git.exe")
	exe := FileStat{Exec: true}
	cases := []struct {
		name   string
		goos   string
		onPath string
		files  []string
		env    map[string]string
		clt    bool
		want   string
	}{
		{"linux on PATH", "linux", "/usr/bin/git", nil, nil, false, "/usr/bin/git"},
		{"linux minimal PATH", "linux", "", []string{"/usr/local/bin/git"}, nil, false, "/usr/local/bin/git"},
		{"linux none", "linux", "", nil, nil, false, ""},
		{"mac stub without tools", "darwin", macGitStub, []string{macGitStub}, nil, false, ""},
		{"mac stub with tools", "darwin", macGitStub, []string{macGitStub}, nil, true, macGitStub},
		{"mac homebrew from Finder", "darwin", "", []string{"/opt/homebrew/bin/git", macGitStub}, nil, false, "/opt/homebrew/bin/git"},
		{"mac stub on PATH, homebrew present", "darwin", macGitStub, []string{"/usr/local/bin/git"}, nil, false, "/usr/local/bin/git"},
		{"windows on PATH", "windows", `C:\git\git.exe`, nil, nil, false, `C:\git\git.exe`},
		{"windows program files", "windows", "", []string{winGit}, map[string]string{"ProgramFiles": `C:\Program Files`}, false, winGit},
		{"windows user install", "windows", "", []string{userGit}, map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`}, false, userGit},
		{"windows none", "windows", "", nil, map[string]string{"ProgramFiles": `C:\Program Files`}, false, ""},
	}
	for _, c := range cases {
		m := newFedora()
		m.goos = c.goos
		m.env = c.env
		if c.onPath != "" {
			m.onPath["git"] = c.onPath
		}
		for _, f := range c.files {
			m.files[f] = exe
		}
		if c.clt {
			m.commands["xcode-select -p"] = fakeOut{out: "/Library/Developer/CommandLineTools\n"}
		} else {
			m.commands["xcode-select -p"] = fakeOut{err: errExit}
		}
		if got := Resolve(context.Background(), m.system(), "git"); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveAdbFromAndroidHome(t *testing.T) {
	m := newFedora()
	m.env = map[string]string{"ANDROID_HOME": "/sdk"}
	m.files["/sdk/platform-tools/adb"] = FileStat{Exec: true}
	m.files["/home/u/Android/Sdk/platform-tools/adb"] = FileStat{Exec: true}
	if got := Resolve(context.Background(), m.system(), "adb"); got != "/sdk/platform-tools/adb" {
		t.Errorf("got %q", got)
	}
}

func TestResolveSkipsNonExecutable(t *testing.T) {
	m := newFedora()
	m.files["/usr/bin/adb"] = FileStat{}
	if got := Resolve(context.Background(), m.system(), "adb"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestPathKeepsAFoundToolAndRetriesAMiss(t *testing.T) {
	m := newFedora()
	old := pathSystem
	pathSystem = m.system
	t.Cleanup(func() {
		pathSystem = old
		pathMu.Lock()
		clear(found)
		pathMu.Unlock()
	})

	if _, err := Path("adb"); err == nil || !strings.HasPrefix(err.Error(), "osdeps: ") {
		t.Fatalf("missing adb err = %v", err)
	}
	m.files["/usr/bin/adb"] = FileStat{Exec: true}
	if p, err := Path("adb"); err != nil || p != "/usr/bin/adb" {
		t.Fatalf("installed adb = %q, %v", p, err)
	}
	m.files["/usr/local/bin/adb"] = FileStat{Exec: true}
	if p, _ := Path("adb"); p != "/usr/bin/adb" {
		t.Errorf("a kept adb moved to %q", p)
	}
	delete(m.files, "/usr/bin/adb")
	if p, _ := Path("adb"); p != "/usr/local/bin/adb" {
		t.Errorf("a removed adb was kept: %q", p)
	}
}
