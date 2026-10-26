// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"encoding/json"
	"os"

	"github.com/faizalv/lemongrass/internal/session"
)

type jsonMessage struct {
	ID        int64  `json:"id"`
	TabID     string `json:"tabId"`
	Label     string `json:"label"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type jsonThread struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	GroupID  int64         `json:"groupId"`
	Messages []jsonMessage `json:"messages"`
	More     bool          `json:"more"`
}

// A read for the app: oldest message first, no read marks and no cursor moves, and sender labels resolved from the group's members.
func printThreadJSON(store *session.Store, threadID string, parsed threadArgs) {
	thread, err := store.ThreadByID(threadID)
	if err != nil {
		fail(err)
	}
	msgs, more, err := store.ReadThread(threadID, parsed.before, parsed.limit)
	if err != nil {
		fail(err)
	}
	labels := map[string]string{}
	if thread.GroupID != 0 {
		members, err := store.GroupMembers(thread.GroupID)
		if err != nil {
			fail(err)
		}
		for _, m := range members {
			labels[m.TabID] = m.Label
		}
	}
	out := jsonThread{ID: thread.ID, Title: thread.Title, GroupID: thread.GroupID, Messages: make([]jsonMessage, 0, len(msgs)), More: more}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		label := labels[m.TabID]
		if label == "" {
			label = session.TabLabel(m.TabID)
		}
		out.Messages = append(out.Messages, jsonMessage{ID: m.ID, TabID: m.TabID, Label: label, Body: m.Body, CreatedAt: m.CreatedAt})
	}
	json.NewEncoder(os.Stdout).Encode(out)
}
