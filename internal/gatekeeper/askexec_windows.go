// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package gatekeeper

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/gitbash"
	"github.com/faizalv/lemongrass/internal/procjob"
)

// conPTY is a Windows pseudoconsole and the two pipes the command's terminal I/O crosses.
type conPTY struct {
	hpc       windows.Handle
	in        *os.File
	out       *os.File
	closeOnce sync.Once
}

// Reading ends with an error once the console closes, which ends the pump.
func (c *conPTY) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *conPTY) Write(p []byte) (int, error) { return c.in.Write(p) }

func (c *conPTY) Resize(rows, cols int) error {
	return windows.ResizePseudoConsole(c.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

// closeConsole ends the console host, which ends the output pipe, so the pump drains what is left and stops.
func (c *conPTY) closeConsole() {
	c.closeOnce.Do(func() { windows.ClosePseudoConsole(c.hpc) })
}

type windowsAskProcess struct {
	pty     *conPTY
	job     *procjob.Job
	process windows.Handle
	code    uint32
}

// startAsk runs the command in Git Bash, as Claude Code's Bash tool does on Windows, inside a ConPTY. The process
// starts suspended and joins a kill-on-close job before it runs, so a stop reaches every process it starts and none
// outlives the daemon, as the process group and parent-death signal do on Linux.
func startAsk(command, dir string, env []string, size askSize) (askProcess, error) {
	bash, err := gitbash.Bash()
	if err != nil {
		return nil, err
	}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, fmt.Errorf("gatekeeper: terminal input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, fmt.Errorf("gatekeeper: terminal output pipe: %w", err)
	}
	// The console host holds its own copies of its pipe ends once it exists, so ours close either way.
	defer windows.CloseHandle(inR)
	defer windows.CloseHandle(outW)
	pty := &conPTY{in: os.NewFile(uintptr(inW), "conpty-in"), out: os.NewFile(uintptr(outR), "conpty-out")}
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(size.cols), Y: int16(size.rows)}, inR, outW, 0, &pty.hpc); err != nil {
		pty.in.Close()
		pty.out.Close()
		return nil, fmt.Errorf("gatekeeper: creating the pseudoconsole: %w", err)
	}
	proc, err := startInConsole(pty, bash, command, dir, env)
	if err != nil {
		pty.closeConsole()
		pty.in.Close()
		pty.out.Close()
		return nil, err
	}
	return proc, nil
}

func startInConsole(pty *conPTY, bash, command, dir string, env []string) (*windowsAskProcess, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself, not a pointer to it; the double indirection keeps go vet's
	// unsafeptr check quiet about converting a handle.
	hpc := pty.hpc
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return nil, fmt.Errorf("gatekeeper: attaching the pseudoconsole: %w", err)
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Without explicit, empty standard handles the command would inherit the daemon's log file as its output.
	si.Flags = windows.STARTF_USESTDHANDLES

	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{bash, "-c", command}))
	if err != nil {
		return nil, err
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, err
	}
	block, err := envBlock(env)
	if err != nil {
		return nil, err
	}
	job, err := procjob.New()
	if err != nil {
		return nil, err
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(nil, cmdline, nil, nil, false, flags, &block[0], cwd, &si.StartupInfo, &pi); err != nil {
		job.Close()
		return nil, fmt.Errorf("gatekeeper: starting %s: %w", bash, err)
	}
	defer windows.CloseHandle(pi.Thread)
	if err := job.Assign(pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		job.Close()
		return nil, err
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		job.Terminate()
		windows.CloseHandle(pi.Process)
		job.Close()
		return nil, fmt.Errorf("gatekeeper: resuming the command: %w", err)
	}
	return &windowsAskProcess{pty: pty, job: job, process: pi.Process}, nil
}

// envBlock is env as CreateProcess wants it: NUL-separated, sorted by name without case, ending in two NULs.
func envBlock(env []string) ([]uint16, error) {
	sorted := slices.Clone(env)
	slices.SortStableFunc(sorted, func(a, b string) int {
		an, _, _ := strings.Cut(a, "=")
		bn, _, _ := strings.Cut(b, "=")
		return strings.Compare(strings.ToUpper(an), strings.ToUpper(bn))
	})
	var block []uint16
	for _, kv := range sorted {
		if strings.IndexByte(kv, 0) >= 0 {
			return nil, errors.New("gatekeeper: environment variable holds a NUL byte")
		}
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	if len(block) == 0 {
		block = append(block, 0)
	}
	return append(block, 0), nil
}

func (p *windowsAskProcess) console() askConsole { return p.pty }

func (p *windowsAskProcess) wait() error {
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		return err
	}
	return windows.GetExitCodeProcess(p.process, &p.code)
}

// Windows has no SIGTERM; a Ctrl+C typed into the console is the polite stop, which ConPTY turns into CTRL_C_EVENT
// for every process attached to it. Killing ends the whole job.
func (p *windowsAskProcess) signal(kill bool) {
	if kill {
		_ = p.job.Terminate()
		return
	}
	_, _ = p.pty.Write([]byte{0x03})
}

func (p *windowsAskProcess) exited() { p.pty.closeConsole() }

func (p *windowsAskProcess) close() {
	p.pty.closeConsole()
	_ = p.pty.in.Close()
	_ = p.pty.out.Close()
	_ = p.job.Close()
	_ = windows.CloseHandle(p.process)
}

func (p *windowsAskProcess) exitCode(waitErr error) int {
	if waitErr != nil {
		return -1
	}
	return int(p.code)
}

// A ConPTY sends screen updates, not a stream of lines, so its tail is read off a screen of the terminal's size.
func newAskTail(size askSize) askTail { return newScreenTail(askTailLines, size) }

// Windows environment names compare without case: PATH and Path are one variable.
func sameEnvName(a, b string) bool { return strings.EqualFold(a, b) }
