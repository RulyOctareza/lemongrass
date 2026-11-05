// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/faizalv/lemongrass/internal/atomicfile"
	"github.com/faizalv/lemongrass/internal/config"
)

const (
	taskName     = `\Lemongrass\lgrassconf`
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "Lemongrass keeper"
	stopTimeout  = 10 * time.Second
)

func taskXMLPath(home string) string {
	return filepath.Join(home, ".lemongrass", "keeper-task.xml")
}

// keeperStampPath holds the sha256 of the binary the running keeper started from, so install can tell whether the
// app copied in a different build.
func keeperStampPath(home string) string {
	return filepath.Join(home, ".lemongrass", "keeper-binary")
}

// installService runs on every packaged app launch. Unlike systemd's restart on Linux it leaves a healthy keeper
// running when its binary is unchanged, since ending and starting a logon task on every launch is slow and visible in
// Task Manager.
func installService(home string) error {
	binary, err := resolveBinary()
	if err != nil {
		return err
	}
	var errs []error
	if err := addUserPath(home, config.PathBinDir(home)); err != nil {
		errs = append(errs, fmt.Errorf("adding %s to PATH: %w", config.PathBinDir(home), err))
	}
	viaTask, err := registerAutostart(home, binary)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	if keeperRunning() {
		if sameBinary(home, binary) {
			return errors.Join(errs...)
		}
		if err := stopKeeper(viaTask); err != nil {
			return errors.Join(append(errs, err)...)
		}
	}
	if err := startKeeper(binary, viaTask); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func uninstallService(home string) error {
	var errs []error
	if keeperRunning() {
		if err := stopKeeper(true); err != nil {
			errs = append(errs, err)
		}
	}
	if _, err := runHidden("schtasks", "/Query", "/TN", taskName); err == nil {
		if out, err := runHidden("schtasks", "/Delete", "/TN", taskName, "/F"); err != nil {
			errs = append(errs, fmt.Errorf("schtasks /Delete: %w: %s", err, out))
		}
	}
	if err := deleteRunValue(); err != nil {
		errs = append(errs, err)
	}
	for _, path := range []string{taskXMLPath(home), keeperStampPath(home)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := removeUserPath(home); err != nil {
		errs = append(errs, fmt.Errorf("removing the PATH entry: %w", err))
	}
	return errors.Join(errs...)
}

// registerAutostart registers the logon task and reports whether it is in place. When Task Scheduler refuses a
// standard user, the keeper starts from the Run key instead; every app launch also runs install, which brings the
// keeper back, so the Run key's lack of a restart on failure costs little.
func registerAutostart(home, binary string) (bool, error) {
	sid, err := currentUserSID()
	if err != nil {
		return false, err
	}
	want := utf16File(taskXML(binary, sid))
	have, _ := os.ReadFile(taskXMLPath(home))
	if bytes.Equal(have, want) {
		if _, err := runHidden("schtasks", "/Query", "/TN", taskName); err == nil {
			return true, deleteRunValue()
		}
	}
	if err := atomicfile.Write(taskXMLPath(home), want, 0o600); err != nil {
		return false, err
	}
	out, err := runHidden("schtasks", "/Create", "/TN", taskName, "/XML", taskXMLPath(home), "/F")
	if err == nil {
		return true, deleteRunValue()
	}
	fmt.Fprintf(os.Stderr, "lgrassconf: the logon task was refused, starting the keeper from the Run key instead: %v: %s\n", err, out)
	_ = os.Remove(taskXMLPath(home))
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer key.Close()
	return false, key.SetStringValue(runValueName, syscall.EscapeArg(binary)+" run")
}

func deleteRunValue() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(runValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// taskXML is a logon task for this user alone, run with the user's interactive token and least privilege, restarted
// every minute after a failure and never stopped for running long or on battery.
func taskXML(binary, sid string) string {
	esc := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Keeps the Lemongrass agent configuration in place.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%[2]s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%[2]s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%[1]s</Command>
      <Arguments>run</Arguments>
    </Exec>
  </Actions>
</Task>
`, esc(binary), esc(sid))
}

// utf16File encodes the task XML as UTF-16LE with a byte order mark, the encoding schtasks /XML reads reliably.
func utf16File(s string) []byte {
	units := utf16.Encode([]rune(strings.ReplaceAll(s, "\n", "\r\n")))
	out := make([]byte, 2, 2+2*len(units))
	out[0], out[1] = 0xFF, 0xFE
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func startKeeper(binary string, viaTask bool) error {
	if viaTask {
		if out, err := runHidden("schtasks", "/Run", "/TN", taskName); err != nil {
			return fmt.Errorf("schtasks /Run: %w: %s", err, out)
		}
		return nil
	}
	cmd := exec.Command(binary, "run")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// stopKeeper asks the running keeper to stop through its stop event, so its deferred cleanup runs, and waits for it to
// let go of its instance mutex. Ending the task is the fallback for a keeper that does not answer.
func stopKeeper(viaTask bool) error {
	if err := signalKeeperStop(); err != nil {
		return err
	}
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if !keeperRunning() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if viaTask {
		runHidden("schtasks", "/End", "/TN", taskName)
		time.Sleep(time.Second)
		if !keeperRunning() {
			return nil
		}
	}
	return errors.New("the running keeper did not stop")
}

func sameBinary(home, binary string) bool {
	stamp, err := os.ReadFile(keeperStampPath(home))
	if err != nil {
		return false
	}
	sum, err := fileSHA256(binary)
	return err == nil && strings.TrimSpace(string(stamp)) == sum
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// runHidden runs a console tool without opening a console window, which a GUI-subsystem lgrassconf would otherwise
// get for every child.
func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
