// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/workgroup"
)

func cmdWorkgroup(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup <create|add|join|disband|thread|list> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		cmdWorkgroupCreate(args[1:])
	case "add":
		cmdWorkgroupAdd(args[1:])
	case "join":
		cmdWorkgroupJoin(args[1:])
	case "disband":
		cmdWorkgroupDisband(args[1:])
	case "thread":
		cmdWorkgroupThread(args[1:])
	case "list":
		cmdWorkgroupList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown workgroup command: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdWorkgroupCreate(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup create <path-to-config>")
		os.Exit(1)
	}
	if tabID == "" {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: creating a group needs a lemongrass tab, %s is not set\n", tabIDEnv)
		os.Exit(1)
	}
	cfg, err := workgroup.Load(args[0])
	if err != nil {
		fail(err)
	}

	if home, err := os.UserHomeDir(); err == nil {
		if err := cfg.CheckSkillsInstalled(home); err != nil {
			fail(err)
		}
	}

	store := openStore()
	defer store.Close()
	if _, err := store.LiveGroupForTab(tabID); err == nil {
		fail(session.ErrAlreadyInside)
	}
	if err := store.CheckGroupName(cfg.Name); err != nil {
		fail(err)
	}
	leaderVendor, _ := store.VendorForTab(tabID)
	proj := currentProject()

	members := make([]workgroup.SpawnMember, len(cfg.Thinkers))
	for i, c := range cfg.Thinkers {
		members[i] = workgroup.SpawnMember{
			TabID:  workgroup.NewTabID(),
			Label:  c.Label,
			Vendor: c.Vendor,
			Model:  c.Model,
			Prompt: workgroup.ComposePrompt(c.Label, cfg.Name, cfg.LeaderLabel, c.RequiredSkills(), c.Prompt),
			Skills: c.RequiredSkills(),
		}
	}
	req := workgroup.Request{ProjectPath: proj.Path, LeaderTabID: tabID, LeaderLabel: cfg.LeaderLabel, GroupName: cfg.Name, Members: members}

	fmt.Println("lgrass: waiting for the human to approve this workgroup in the lemongrass app.")
	answer, err := workgroup.Propose(workgroup.AppSocketPath(), req)
	if err != nil {
		fail(err)
	}
	if !answer.Approved {
		fmt.Fprintln(os.Stderr, declinedMessage(answer))
		os.Exit(1)
	}
	token := answer.Token

	group, err := store.CreateGroup(cfg.Name, session.Member{TabID: tabID, Label: cfg.LeaderLabel, Vendor: leaderVendor}, toMembers(members, cfg.Thinkers))
	if err != nil {
		fail(err)
	}
	if err := workgroup.Spawn(workgroup.AppSocketPath(), token); err != nil {
		store.DeleteGroup(group.ID)
		fail(err)
	}

	fmt.Printf("workgroup %d created, group thread %s.\n", group.ID, group.ThreadID)
	for _, m := range members {
		fmt.Printf("  %s (%s) tab %s\n", m.Label, m.Vendor, m.TabID)
	}
}

func cmdWorkgroupAdd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup add <path-to-config>")
		os.Exit(1)
	}
	if tabID == "" {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: adding thinkers needs a lemongrass tab, %s is not set\n", tabIDEnv)
		os.Exit(1)
	}

	store := openStore()
	defer store.Close()
	group, me, err := store.LiveMembership(tabID)
	if err != nil || me.Role != session.RoleLeader {
		fmt.Fprintln(os.Stderr, "lgrass workgroup: only the leader of a live workgroup can add thinkers to it")
		os.Exit(1)
	}
	current, err := store.GroupMembers(group.ID)
	if err != nil {
		fail(err)
	}
	labels := make([]string, len(current))
	for i, m := range current {
		labels[i] = m.Label
	}

	cfg, err := workgroup.LoadAddition(args[0], labels)
	if err != nil {
		fail(err)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if err := cfg.CheckSkillsInstalled(home); err != nil {
			fail(err)
		}
	}

	members := make([]workgroup.SpawnMember, len(cfg.Thinkers))
	for i, c := range cfg.Thinkers {
		members[i] = workgroup.SpawnMember{
			TabID:  workgroup.NewTabID(),
			Label:  c.Label,
			Vendor: c.Vendor,
			Model:  c.Model,
			Prompt: workgroup.ComposeAddedPrompt(c.Label, group.Name, me.Label, c.RequiredSkills(), c.Prompt),
			Skills: c.RequiredSkills(),
		}
	}
	req := workgroup.Request{ProjectPath: currentProject().Path, LeaderTabID: tabID, LeaderLabel: me.Label, GroupName: group.Name, GroupID: group.ID, ExistingMembers: labels, Members: members}

	fmt.Println("lgrass: waiting for the human to approve the new thinkers in the lemongrass app.")
	answer, err := workgroup.Propose(workgroup.AppSocketPath(), req)
	if err != nil {
		fail(err)
	}
	if !answer.Approved {
		fmt.Fprintln(os.Stderr, declinedMessage(answer))
		os.Exit(1)
	}

	if err := store.AddMembers(group.ID, toMembers(members, cfg.Thinkers)); err != nil {
		fail(err)
	}
	if err := workgroup.Spawn(workgroup.AppSocketPath(), answer.Token); err != nil {
		tabs := make([]string, len(members))
		for i, m := range members {
			tabs[i] = m.TabID
		}
		store.DeleteMembers(group.ID, tabs)
		fail(err)
	}
	if _, err := store.PostSystemMessage(group.ThreadID, joinMessage(members)); err == nil {
		notifyTargets(store)
	}

	fmt.Printf("added to workgroup %d, group thread %s.\n", group.ID, group.ThreadID)
	for _, m := range members {
		fmt.Printf("  %s (%s) tab %s\n", m.Label, m.Vendor, m.TabID)
	}
}

// Run by the lemongrass app after the human approves adding an open tab to a live group. The token is minted by the app on that approval and redeemed here once, so no model can make a tab join.
func cmdWorkgroupJoin(args []string) {
	token, asJSON := "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--token":
			if i+1 < len(args) {
				i++
				token = args[i]
			}
		case "--json":
			asJSON = true
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup join --token <token> [--json]")
		os.Exit(1)
	}
	grant, err := workgroup.Claim(workgroup.AppSocketPath(), token)
	if err != nil {
		fail(err)
	}
	if grant.ProjectPath != currentProject().Path {
		fmt.Fprintln(os.Stderr, "lgrass workgroup: the approved join belongs to another project")
		os.Exit(1)
	}
	store := openStore()
	defer store.Close()
	group, err := store.GroupByID(grant.GroupID)
	if err != nil {
		fail(err)
	}
	if !group.Live() {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: workgroup %d is disbanded\n", group.ID)
		os.Exit(1)
	}
	vendor, _ := store.VendorForTab(grant.TabID)
	if vendor != "claude" && vendor != "codex" {
		fmt.Fprintln(os.Stderr, "lgrass workgroup: only a Claude or Codex tab can join a workgroup")
		os.Exit(1)
	}
	if _, err := store.LiveGroupForTab(grant.TabID); err == nil {
		fail(session.ErrAlreadyInside)
	}
	// A tab title is written by a model, so the label models read is the short tab id; the app shows the title to the human.
	label := session.ShortID(grant.TabID)
	members, err := store.GroupMembers(group.ID)
	if err != nil {
		fail(err)
	}
	for _, m := range members {
		if strings.EqualFold(m.Label, label) {
			fail(fmt.Errorf("%w (%q)", session.ErrLabelTaken, label))
		}
	}

	// Posted before the row exists, so the joining tab is not woken by its own arrival; the app types its first prompt.
	if _, err := store.PostSystemMessage(group.ThreadID, collaboratorJoinMessage(label, vendor)); err != nil {
		fail(err)
	}
	if err := store.AddCollaborator(group.ID, session.Member{TabID: grant.TabID, Label: label, Vendor: vendor}); err != nil {
		fail(err)
	}
	notifyTargets(store)

	if asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"group_id": group.ID, "thread_id": group.ThreadID, "label": label})
		return
	}
	fmt.Printf("%s joined workgroup %d as a collaborator, group thread %s.\n", label, group.ID, group.ThreadID)
}

func collaboratorJoinMessage(label, vendor string) string {
	return fmt.Sprintf("%s (%s) joined as a collaborator. It keeps its own context and work, and the human directs it in its own tab.", label, vendor)
}

// Posted by lemongrass with no mention, so every member hears it, the leader and the new thinkers included.
func joinMessage(members []workgroup.SpawnMember) string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = fmt.Sprintf("%s (%s)", m.Label, m.Vendor)
	}
	joins := " joins the group as a new thinker"
	if len(members) > 1 {
		joins = " join the group as new thinkers"
	}
	return strings.Join(names, ", ") + joins + ", approved by the human."
}

func toMembers(spawn []workgroup.SpawnMember, thinkers []workgroup.Thinker) []session.Member {
	out := make([]session.Member, len(spawn))
	for i, m := range spawn {
		out[i] = session.Member{TabID: m.TabID, Label: m.Label, Vendor: m.Vendor, Prompt: thinkers[i].Prompt, Skills: thinkers[i].Skills}
	}
	return out
}

func cmdWorkgroupDisband(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup disband <workgroup-id>")
		os.Exit(1)
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: %q is not a workgroup id\n", args[0])
		os.Exit(1)
	}

	store := openStore()
	defer store.Close()
	group, err := store.GroupByID(id)
	if err != nil {
		fail(err)
	}
	if tabID != "" && tabID != group.LeaderTabID {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: only the leader can disband workgroup %d\n", id)
		os.Exit(1)
	}
	if err := store.DisbandGroup(id); err != nil {
		fail(err)
	}
	fmt.Printf("workgroup %d disbanded. Its tabs stay open and its thread stays readable.\n", id)
}

func cmdWorkgroupThread(args []string) {
	parsed := parseThreadArgs(args, defaultThreadReadLimit)
	if tabID == "" {
		fmt.Fprintf(os.Stderr, "lgrass workgroup: this needs a lemongrass tab, %s is not set\n", tabIDEnv)
		os.Exit(1)
	}

	store := openStore()
	defer store.Close()
	group, err := store.LiveGroupForTab(tabID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lgrass workgroup: this tab is not in a live workgroup")
		os.Exit(1)
	}
	members, err := store.GroupMembers(group.ID)
	if err != nil {
		fail(err)
	}
	var self session.Member
	for _, m := range members {
		if m.TabID == tabID {
			self = m
		}
	}
	header := func(firstRead bool) string {
		if !firstRead {
			return session.FormatGroupShort(group, self)
		}
		return session.FormatGroupHeader(group, members) + "\n" + session.FormatMemberHeader(self)
	}
	printThread(store, group.ThreadID, parsed, header)
}

type listedMember struct {
	TabID  string `json:"tabId"`
	Role   string `json:"role"`
	Label  string `json:"label"`
	Vendor string `json:"vendor"`
}

type listedGroup struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	ThreadID    string         `json:"threadId"`
	LeaderTabID string         `json:"leaderTabId"`
	Members     []listedMember `json:"members"`
}

func cmdWorkgroupList(args []string) {
	asJSON := len(args) == 1 && args[0] == "--json"
	if len(args) > 1 || (len(args) == 1 && !asJSON) {
		fmt.Fprintln(os.Stderr, "usage: lgrass workgroup list [--json]")
		os.Exit(1)
	}

	store := openStore()
	defer store.Close()
	groups, err := store.LiveGroups()
	if err != nil {
		fail(err)
	}
	listed := make([]listedGroup, 0, len(groups))
	for _, g := range groups {
		members, err := store.GroupMembers(g.ID)
		if err != nil {
			fail(err)
		}
		entry := listedGroup{ID: g.ID, Name: g.Name, ThreadID: g.ThreadID, LeaderTabID: g.LeaderTabID, Members: make([]listedMember, len(members))}
		for i, m := range members {
			entry.Members[i] = listedMember{TabID: m.TabID, Role: m.Role, Label: m.Label, Vendor: m.Vendor}
		}
		listed = append(listed, entry)
	}

	if asJSON {
		json.NewEncoder(os.Stdout).Encode(listed)
		return
	}
	if len(listed) == 0 {
		fmt.Println("no live workgroups in this project")
		return
	}
	for _, g := range listed {
		fmt.Printf("workgroup %d [%s], thread %s\n", g.ID, g.Name, g.ThreadID)
		for _, m := range g.Members {
			fmt.Printf("  %s (%s, %s) tab %s\n", m.Label, m.Role, m.Vendor, m.TabID)
		}
	}
}

func declinedMessage(answer workgroup.Answer) string {
	switch {
	case answer.Withdrawn:
		return "lgrass workgroup: the proposal was withdrawn before the human answered, nothing was created"
	case answer.Reason != "":
		return fmt.Sprintf("lgrass workgroup: the human declined, nothing was created. Their reason: %s", answer.Reason)
	default:
		return "lgrass workgroup: the human declined without a reason, nothing was created"
	}
}
