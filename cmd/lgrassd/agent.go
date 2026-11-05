// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"fmt"
	"github.com/faizalv/lemongrass/internal/ipc"
	"os"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/bibliohistory"
	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/threadsvc"
	"github.com/faizalv/lemongrass/internal/vault"
)

const (
	agentQueryMaxFailures = 5
	agentQueryWindow      = time.Minute
	agentQueryLockout     = 5 * time.Minute

	threadRetryInterval = 30 * time.Second
	threadPruneInterval = time.Hour

	historyDebounce = 5 * time.Second
	historyPass     = 10 * time.Minute
	historyKeep     = 90 * 24 * time.Hour
	historyPrune    = 24 * time.Hour
)

func cmdAgent(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd agent run")
		os.Exit(1)
	}
	switch args[0] {
	case "run":
		cmdAgentRun()
	default:
		fmt.Fprintf(os.Stderr, "unknown agent command: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdAgentRun() {
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sockPath := agent.SocketPath()
	l, err := ipc.Listen(sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer l.Close()

	vaultClient := &gatekeeper.Client{SocketPath: gatekeeper.SocketPath()}
	svc := agent.NewService(vaultClient)
	limiter := vault.NewFailureLimiter(agentQueryMaxFailures, agentQueryWindow, agentQueryLockout)

	startThreadService()
	go runHistoryWatcher()

	fmt.Printf("lgrassd agent: listening on %s\n", sockPath)
	if err := agent.Serve(svc, l, limiter); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// A failure here is logged and never stops the agent, since the db and rester proxy do not depend on threads.
func startThreadService() {
	store, err := session.Open(session.DBPath(), "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd agent: thread service not started: %v\n", err)
		return
	}
	threadSock := threadsvc.SocketPath()
	l, err := ipc.Listen(threadSock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd agent: thread service not started: %v\n", err)
		store.Close()
		return
	}
	svc := threadsvc.NewService(store)
	go svc.Serve(l)
	go svc.Run(context.Background(), threadRetryInterval, threadPruneInterval)
	fmt.Printf("lgrassd agent: thread service listening on %s\n", threadSock)
}

// Runs inside the agent daemon, so it lives only while the app runs; its pass at start commits what changed while the
// app was closed. A failure is logged and never stops the agent.
func runHistoryWatcher() {
	w := &bibliohistory.Watcher{
		Projects: project.List,
		GitDir:   bibliohistory.GitDir,
		Sessions: historySessions,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "lgrassd agent: "+format+"\n", args...)
		},
		Debounce:   historyDebounce,
		Pass:       historyPass,
		Keep:       historyKeep,
		PruneEvery: historyPrune,
	}
	if err := w.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd agent: biblio history not started: %v\n", err)
	}
}

func historySessions(p project.Project, paths []string, since time.Time) []string {
	store, err := session.Open(session.DBPath(), p.ID)
	if err != nil {
		return nil
	}
	defer store.Close()
	touches, err := store.FileActivitySince(since)
	if err != nil {
		return nil
	}
	return bibliohistory.Attribute(p.Path, touches, paths)
}
