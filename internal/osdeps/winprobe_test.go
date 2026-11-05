// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

const (
	winLocal   = `C:\Users\a\AppData\Local`
	winWinget  = winLocal + `\Microsoft\WindowsApps\winget.exe`
	winPkgs    = winLocal + `\Microsoft\WinGet\Packages`
	winPlatAdb = winPkgs + `\Google.PlatformTools_Microsoft.Winget.Source_8wekyb3d8bbwe\platform-tools\adb.exe`
	winScrcpy  = winPkgs + `\Genymobile.scrcpy_Microsoft.Winget.Source_8wekyb3d8bbwe\scrcpy-win64-v3.3\scrcpy.exe`
)

// newWindows is a Windows 11 machine with winget and nothing else installed.
func newWindows() *fakeMachine {
	m := newFedora()
	m.goos = "windows"
	m.home = `C:\Users\a`
	m.osRelease = ""
	m.env = map[string]string{
		"LOCALAPPDATA": winLocal,
		"ProgramFiles": `C:\Program Files`,
		"SystemRoot":   `C:\Windows`,
	}
	m.commands["cmd /c ver"] = fakeOut{out: "\r\nMicrosoft Windows [Version 10.0.22631.4460]\r\n"}
	m.onPath["winget"] = winWinget
	return m
}

// installWin puts a binary on PATH with its version output; Windows has no package database to fake.
func (m *fakeMachine) installWin(name, path, versionOut string) {
	m.files[path] = FileStat{Size: 100, ModTime: 1, Exec: true}
	m.onPath[name] = path
	d, _ := Find(name)
	m.commands[path+" "+strings.Join(d.VersionArgs, " ")] = fakeOut{out: versionOut}
}

func checkReport(t *testing.T, m *fakeMachine) Report {
	t.Helper()
	report, err := Check(context.Background(), m.system(), filepath.Join(t.TempDir(), "deps.json"), false)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return report
}

func TestParseWindowsVer(t *testing.T) {
	cases := []struct {
		out     string
		version string
		build   int
	}{
		{"Microsoft Windows [Version 10.0.22631.4460]", "11 (build 22631)", 22631},
		{"Microsoft Windows [Version 10.0.19045.5011]", "10 (build 19045)", 19045},
		{"Microsoft Windows [Versión 10.0.26100.2033]", "11 (build 26100)", 26100},
		{"garbage", "", 0},
	}
	for _, c := range cases {
		got := ParseWindowsVer(c.out)
		if got.Family != FamilyWindows || got.Version != c.version || got.Build != c.build {
			t.Errorf("%q: got %+v", c.out, got)
		}
	}
}

func TestCheckWindowsFreshMachine(t *testing.T) {
	m := newWindows()
	m.installWin("ssh", `C:\Windows\System32\OpenSSH\ssh.exe`, "OpenSSH_for_Windows_9.5p1, LibreSSL 3.8.2\r\n")
	report := checkReport(t, m)
	if report.OS.Version != "11 (build 22631)" || !report.OS.Winget {
		t.Errorf("os = %+v", report.OS)
	}
	want := map[string]string{"git": "Git.Git", "adb": "Google.PlatformTools", "scrcpy": "Genymobile.scrcpy", "ffmpeg": "Gyan.FFmpeg"}
	for _, d := range report.Deps {
		if pkg, ok := want[d.Name]; ok {
			if d.Status != StatusMissing || d.Offer == nil || *d.Offer != (Offer{ActionInstall, pkg, RouteWinget}) {
				t.Errorf("%s = %+v", d.Name, d)
			}
			continue
		}
		if d.Status != StatusOK || d.Version != "9.5p1" || d.Offer != nil ||
			d.Origin != (Origin{Kind: OriginPackage, Package: "OpenSSH.Client", Manager: ManagerWindows}) {
			t.Errorf("ssh = %+v", d)
		}
	}
	if m.callCount("rpm ") != 0 {
		t.Errorf("rpm ran on Windows")
	}
}

func TestCheckWindowsWithoutWinget(t *testing.T) {
	m := newWindows()
	delete(m.onPath, "winget")
	got := check(t, m)
	if git := got["git"]; git.Offer != nil || !strings.Contains(git.Message, "App Installer") {
		t.Errorf("git = %+v", git)
	}
	if ssh := got["ssh"]; ssh.Offer != nil || !strings.Contains(ssh.Message, "Optional features") {
		t.Errorf("ssh = %+v", ssh)
	}
}

func TestCheckWindowsWingetFromItsAlias(t *testing.T) {
	m := newWindows()
	delete(m.onPath, "winget")
	m.files[winWinget] = FileStat{Exec: true}
	if adb := check(t, m)["adb"]; adb.Offer == nil {
		t.Errorf("adb = %+v", adb)
	}
}

func TestCheckWindowsOrigins(t *testing.T) {
	cases := []struct {
		name, path, link, versionOut string
		want                         Origin
	}{
		{"adb", winPlatAdb, "", "Version 37.0.1-1\r\n",
			Origin{Kind: OriginPackage, Package: "Google.PlatformTools", Manager: ManagerWinget}},
		{"ffmpeg", winLocal + `\Microsoft\WinGet\Links\ffmpeg.exe`,
			winPkgs + `\Gyan.FFmpeg_Microsoft.Winget.Source_8wekyb3d8bbwe\ffmpeg-8.0-full_build\bin\ffmpeg.exe`,
			"ffmpeg version 8.0-full_build-www.gyan.dev\r\n",
			Origin{Kind: OriginPackage, Package: "Gyan.FFmpeg", Manager: ManagerWinget}},
		{"git", `C:\PROGRAM FILES\Git\cmd\git.exe`, "", "git version 2.56.0.windows.4\r\n",
			Origin{Kind: OriginPackage, Package: "Git.Git", Manager: ManagerInstaller}},
		{"git", winLocal + `\Programs\Git\cmd\git.exe`, "", "git version 2.56.0.windows.4\r\n",
			Origin{Kind: OriginPackage, Package: "Git.Git", Manager: ManagerInstaller}},
		{"scrcpy", `C:\Users\a\scoop\apps\scrcpy\current\scrcpy.exe`, "", "scrcpy 3.3\r\n",
			Origin{Kind: OriginPackage, Package: "scrcpy", Manager: ManagerScoop}},
		{"adb", `C:\platform-tools\adb.exe`, "", "Version 37.0.1-1\r\n", Origin{Kind: OriginManual}},
	}
	for _, c := range cases {
		m := newWindows()
		m.installWin(c.name, c.path, c.versionOut)
		if c.link != "" {
			m.links[c.path] = c.link
		}
		if got := check(t, m)[c.name]; got.Origin != c.want || got.Status != StatusOK || got.Offer != nil {
			t.Errorf("%s at %s = %+v", c.name, c.path, got)
		}
	}
}

func TestCheckWindowsScoopShim(t *testing.T) {
	m := newWindows()
	shim := `C:\Users\a\scoop\shims\adb.exe`
	m.installWin("adb", shim, "Version 37.0.1-1\r\n")
	sys := m.system()
	read := sys.ReadFile
	sys.ReadFile = func(path string) ([]byte, error) {
		if path == `C:\Users\a\scoop\shims\adb.shim` {
			return []byte("path = \"C:\\Users\\a\\scoop\\apps\\adb\\current\\platform-tools\\adb.exe\"\r\n"), nil
		}
		return read(path)
	}
	report, err := Check(context.Background(), sys, filepath.Join(t.TempDir(), "deps.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if adb := report.Deps[1]; adb.Origin != (Origin{Kind: OriginPackage, Package: "adb", Manager: ManagerScoop}) {
		t.Errorf("adb = %+v", adb)
	}
}

func TestCheckWindowsGitBelowFloor(t *testing.T) {
	cases := []struct {
		path    string
		offer   *Offer
		message string
	}{
		{`C:\Program Files\Git\cmd\git.exe`, &Offer{ActionUpgrade, "Git.Git", RouteWinget}, ""},
		{`C:\Users\a\scoop\apps\git\current\cmd\git.exe`, nil, "scoop update git"},
		{`D:\tools\git\cmd\git.exe`, nil, "installed by hand"},
	}
	for _, c := range cases {
		m := newWindows()
		m.installWin("git", c.path, "git version 2.40.1.windows.1\r\n")
		git := check(t, m)["git"]
		if git.Status != StatusBelowFloor || (c.offer == nil) != (git.Offer == nil) ||
			(c.offer != nil && *git.Offer != *c.offer) || !strings.Contains(git.Message, c.message) {
			t.Errorf("%s: git = %+v", c.path, git)
		}
	}
}

func TestResolveWindowsPrefersPlatformToolsOverScrcpysAdb(t *testing.T) {
	m := newWindows()
	bundled := winDir(winScrcpy) + `\adb.exe`
	m.onPath["adb"] = bundled
	m.files[bundled] = FileStat{Exec: true}
	m.files[winScrcpy] = FileStat{Exec: true}
	if got := Resolve(context.Background(), m.system(), "adb"); got != bundled {
		t.Errorf("with no other adb, got %q", got)
	}
	m.files[winPlatAdb] = FileStat{Exec: true}
	if got := Resolve(context.Background(), m.system(), "adb"); got != winPlatAdb {
		t.Errorf("got %q, want the platform-tools adb", got)
	}
}

func TestResolveWindowsAndroidStudioSdk(t *testing.T) {
	m := newWindows()
	sdk := winLocal + `\Android\Sdk\platform-tools\adb.exe`
	m.files[sdk] = FileStat{Exec: true}
	if got := Resolve(context.Background(), m.system(), "adb"); got != sdk {
		t.Errorf("got %q", got)
	}
}

func TestCheckWindowsBlockedUnsignedTool(t *testing.T) {
	m := newWindows()
	ffmpeg := `C:\tools\ffmpeg.exe`
	m.installWin("ffmpeg", ffmpeg, "")
	m.commands[ffmpeg+" -version"] = fakeOut{err: errExit}
	cache := filepath.Join(t.TempDir(), "deps.json")
	for range 2 {
		report, _ := Check(context.Background(), m.system(), cache, false)
		if ff := report.Deps[3]; !strings.Contains(ff.Message, "Smart App Control") {
			t.Errorf("ffmpeg = %+v", ff)
		}
	}
	if n := m.callCount(ffmpeg); n != 2 {
		t.Errorf("a blocked tool was cached: %d runs", n)
	}
}
