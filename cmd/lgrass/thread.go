// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/threadsvc"
)

const (
	defaultThreadListLimit = 20
	defaultThreadReadLimit = 10
)

type threadArgs struct {
	positional []string
	file       string
	before     int64
	limit      int
	all        bool
	json       bool
}

func parseThreadArgs(args []string, defaultLimit int) threadArgs {
	parsed := threadArgs{limit: defaultLimit}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			i++
			if i < len(args) {
				parsed.file = args[i]
			}
		case "--all":
			parsed.all = true
		case "--json":
			parsed.json = true
		case "--before":
			i++
			if i < len(args) {
				parsed.before, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--limit":
			i++
			if i < len(args) {
				if n, err := strconv.Atoi(args[i]); err == nil && n > 0 {
					parsed.limit = n
				}
			}
		default:
			parsed.positional = append(parsed.positional, args[i])
		}
	}
	return parsed
}

func cmdThread(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass thread <create|post|read|list> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		cmdThreadCreate(args[1:])
	case "post":
		cmdThreadPost(args[1:])
	case "read":
		cmdThreadRead(args[1:])
	case "list":
		cmdThreadList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown thread command: %s\n", args[0])
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func requireTab() {
	if tabID == "" {
		fmt.Fprintf(os.Stderr, "lgrass thread: posting needs a lemongrass tab, %s is not set\n", tabIDEnv)
		os.Exit(1)
	}
}

// Wakes the thread service, or delivers directly and prunes the ledger itself when the service is unreachable.
func notifyTargets(store *session.Store) {
	if threadsvc.Wake(threadsvc.SocketPath()) == nil {
		return
	}
	threadsvc.NewDeliverer(store).DeliverPending()
	store.PruneNotifications(time.Now())
}

func openStore() *session.Store {
	store, err := session.Open(session.DBPath(), currentProject().ID)
	if err != nil {
		fail(err)
	}
	return store
}

// Content is the positional argument, "-" for stdin, or --file, so long or quote-heavy text never has to pass through a shell argument.
func threadContent(parsed threadArgs, positionalIndex int) (string, error) {
	var text string
	switch {
	case parsed.file != "":
		data, err := os.ReadFile(parsed.file)
		if err != nil {
			return "", err
		}
		text = string(data)
	case len(parsed.positional) > positionalIndex && parsed.positional[positionalIndex] == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		text = string(data)
	case len(parsed.positional) > positionalIndex:
		text = parsed.positional[positionalIndex]
	}
	return strings.TrimSpace(text), nil
}

func cmdThreadCreate(args []string) {
	parsed := parseThreadArgs(args, defaultThreadReadLimit)
	if len(parsed.positional) < 1 {
		fmt.Fprintln(os.Stderr, `usage: lgrass thread create "<title>" "<content>" (content may be - for stdin or --file <path>)`)
		os.Exit(1)
	}
	requireTab()
	content, err := threadContent(parsed, 1)
	if err != nil {
		fail(err)
	}

	store := openStore()
	defer store.Close()
	id, err := store.CreateThread(tabID, parsed.positional[0], content)
	if err != nil {
		fail(err)
	}
	notifyTargets(store)
	fmt.Printf("thread %s created.\n", id)
}

func cmdThreadPost(args []string) {
	parsed := parseThreadArgs(args, defaultThreadReadLimit)
	if len(parsed.positional) < 1 {
		fmt.Fprintln(os.Stderr, `usage: lgrass thread post <thread-id> "<content>" (content may be - for stdin or --file <path>)`)
		os.Exit(1)
	}
	threadID := parsed.positional[0]
	requireTab()
	content, err := threadContent(parsed, 1)
	if err != nil {
		fail(err)
	}

	store := openStore()
	defer store.Close()
	msg, err := store.PostMessage(tabID, threadID, content)
	if err != nil {
		fail(err)
	}
	notifyTargets(store)
	fmt.Printf("posted as message %d.\n", msg.ID)
}

func cmdThreadRead(args []string) {
	parsed := parseThreadArgs(args, defaultThreadReadLimit)
	if len(parsed.positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrass thread read <thread-id> [--all] [--before <message-id>] [--limit N] [--json]")
		os.Exit(1)
	}
	threadID := parsed.positional[0]

	store := openStore()
	defer store.Close()
	if parsed.json {
		printThreadJSON(store, threadID, parsed)
		return
	}
	printThread(store, threadID, parsed, nil)
}

// The shared read path. A lemongrass tab that has read the thread before gets only what is new to it, unless it asks for --all or an older page. header gets whether this is the tab's first read of the thread. The tab's notifications for the messages shown are settled afterwards.
func printThread(store *session.Store, threadID string, parsed threadArgs, header func(firstRead bool) string) {
	thread, err := store.ThreadByID(threadID)
	if err != nil {
		fail(err)
	}
	var cursor int64
	seen := false
	if tabID != "" {
		if cursor, seen, err = store.ThreadCursor(tabID, threadID); err != nil {
			fail(err)
		}
	}
	unread := seen && !parsed.all && parsed.before == 0

	var msgs []session.Message
	var more bool
	if unread {
		msgs, more, err = store.ReadUnread(threadID, tabID, cursor, parsed.limit)
	} else {
		msgs, more, err = store.ReadThread(threadID, parsed.before, parsed.limit)
	}
	if err != nil {
		fail(err)
	}
	if tabID != "" {
		ids := make([]int64, len(msgs))
		var newest int64
		for i, m := range msgs {
			ids[i] = m.ID
		}
		if len(msgs) > 0 {
			newest = msgs[0].ID
		}
		store.MarkMessagesRead(tabID, ids)
		if parsed.before == 0 {
			store.AdvanceThreadCursor(tabID, threadID, newest)
		}
	}
	if header != nil {
		fmt.Println(header(!seen || parsed.all))
	}
	if len(msgs) == 0 && unread {
		fmt.Println(session.FormatNothingNew(thread))
		return
	}
	fmt.Println(session.FormatThreadRead(thread, msgs, more, unread, threadLabels(store, thread, msgs)))
}

func cmdThreadList(args []string) {
	parsed := parseThreadArgs(args, defaultThreadListLimit)

	store := openStore()
	defer store.Close()
	threads, err := store.ListThreads(parsed.limit)
	if err != nil {
		fail(err)
	}
	fmt.Println(session.FormatThreadList(threads, store.Labels(threadCreators(threads))))
}

func threadCreators(threads []session.Thread) []string {
	ids := make([]string, len(threads))
	for i, t := range threads {
		ids[i] = t.CreatedBy
	}
	return ids
}

func threadLabels(store *session.Store, thread session.Thread, msgs []session.Message) map[string]string {
	ids := []string{thread.CreatedBy}
	for _, m := range msgs {
		ids = append(ids, m.TabID)
		ids = append(ids, m.Mentions...)
	}
	return store.Labels(ids)
}
