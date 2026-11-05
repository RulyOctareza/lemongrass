// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// An ask is a model's request for something only the human can give. Asks live in daemon memory only, so a daemon restart drops every one of them; nothing about an ask is sealed or written to disk.

const (
	AskDB      = "db"
	AskRester  = "rester"
	AskCommand = "command"
)

const (
	AskWaiting  = "waiting"
	AskRunning  = "running"
	AskAnswered = "answered"
	AskDeclined = "declined"
	AskExpired  = "expired"
)

const (
	MaxAskReason    = 2000
	MaxAskMessage   = 4000
	MaxAskCommand   = 8000
	MaxAskTables    = 50
	MaxAskTableName = 128
	MaxAskShortID   = 64
	// MaxAsks bounds the records held at once, answered ones included until they expire.
	MaxAsks = 200
	// AskLifetime is how long a record is kept after it was filed, answered or not.
	AskLifetime = 24 * time.Hour
	// AskMaxWait is the longest one wait_ask blocks; the ask itself stays open afterwards.
	AskMaxWait = 30 * time.Minute
	// AskAckGrace is how long an outcome handed to a waiter may go unacknowledged before it is sent to the tab as a nudge instead. The waiter's reply passes through the agent to lgrass, so the vault cannot see it arrive; lgrass acks once it has printed it.
	AskAckGrace = 20 * time.Second
	// A tab may file askRateMax asks in askRateWindow and hold askOpenMax open at once.
	askRateMax    = 10
	askRateWindow = 10 * time.Minute
	askOpenMax    = 5
)

// AskDBOperations are the operations a db ask may declare: the strings dbgate.AllowStatement checks a channel's scope for, kept here because dbgate imports this package.
var AskDBOperations = []string{"select", "explain", "show", "insert", "update", "delete", "performance"}

// AskHTTPMethods are the methods a rester ask may declare.
var AskHTTPMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

var (
	ErrNoAsk         = errors.New("vault: no such ask")
	ErrAskNotWaiting = errors.New("vault: the ask is no longer waiting for a decision")
)

// AskAccountEnvKeys name the variables that pick a Claude or Codex account. A command ask carries the caller's values, so a login run from a tab reaches that tab's account.
var AskAccountEnvKeys = []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"}

// AskAccountEnv reads the caller's account variables, leaving out the unset ones; nil when none is set.
func AskAccountEnv(getenv func(string) string) map[string]string {
	var env map[string]string
	for _, k := range AskAccountEnvKeys {
		if v := getenv(k); v != "" {
			if env == nil {
				env = map[string]string{}
			}
			env[k] = v
		}
	}
	return env
}

// AskInput is what a caller files. TabID, Cwd, Path and AccountEnv come from the caller's environment, not from text the model typed.
type AskInput struct {
	Type       string
	TabID      string
	Cwd        string
	Path       string
	AccountEnv map[string]string
	Reason     string
	Tables     []string
	Operations []string
	Message    string
	Methods    []string
	Command    string
	// Rule is the guard rule that refused the call when the hook filed the ask itself.
	Rule string
}

// AskResult is a finished command run. EndedAt is zero while the command still runs.
type AskResult struct {
	ExitCode   int       `json:"exit_code"`
	OutputTail string    `json:"output_tail"`
	TimedOut   bool      `json:"timed_out"`
	Stopped    bool      `json:"stopped"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
}

// AskGrant is what the human granted for a channel ask: never more than the model declared.
type AskGrant struct {
	Tables     []string `json:"tables,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Methods    []string `json:"methods,omitempty"`
}

// RemovedTable is a requested table the human left out of a db ask's channel, with why.
type RemovedTable struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
}

// Ask is one record as the app and the model see it.
type Ask struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	TabID     string    `json:"tab_id"`
	Cwd       string    `json:"cwd"`
	CreatedAt time.Time `json:"created_at"`
	State     string    `json:"state"`
	Blocking  bool      `json:"blocking"`
	Reason    string    `json:"reason,omitempty"`
	Tables    []string  `json:"tables,omitempty"`
	// Operations and Methods are what the model declared for a db or a rester ask.
	Operations []string   `json:"operations,omitempty"`
	Message    string     `json:"message,omitempty"`
	Methods    []string   `json:"methods,omitempty"`
	Command    string     `json:"command,omitempty"`
	Rule       string     `json:"rule,omitempty"`
	ShortID    string     `json:"short_id,omitempty"`
	Note       string     `json:"note,omitempty"`
	Result     *AskResult `json:"result,omitempty"`
	// Granted and RemovedTables are set when a channel ask is answered or declined.
	Granted       *AskGrant      `json:"granted,omitempty"`
	RemovedTables []RemovedTable `json:"removed_tables,omitempty"`
	// Path is the caller's PATH, which an executed command runs with.
	Path string `json:"-"`
	// AccountEnv is the caller's account variables, which an executed command runs with; the card shows them.
	AccountEnv map[string]string `json:"account_env,omitempty"`
}

// Final reports whether the ask has its outcome, so a waiter stops waiting.
func (a Ask) Final() bool {
	return a.State == AskAnswered || a.State == AskDeclined || a.State == AskExpired
}

type askRecord struct {
	Ask
	channel   ChannelID
	waiters   int
	delivered bool
	done      chan struct{}
}

// AskStore holds the asks. OnChange runs after any change the app should refetch for, and OnUndelivered runs when an ask reaches its outcome with no waiter to hand it to, so the outcome goes to the tab as a nudge. Both run outside the store's lock.
type AskStore struct {
	OnChange      func()
	OnUndelivered func(Ask)

	mu      sync.Mutex
	asks    map[string]*askRecord
	filings map[string][]time.Time
	now     func() time.Time
	grace   time.Duration
}

func NewAskStore() *AskStore {
	return &AskStore{asks: map[string]*askRecord{}, filings: map[string][]time.Time{}, now: time.Now, grace: AskAckGrace}
}

type askEvents struct {
	changed     bool
	undelivered []Ask
}

func (s *AskStore) fire(ev askEvents) {
	for _, a := range ev.undelivered {
		if s.OnUndelivered != nil {
			s.OnUndelivered(a)
		}
	}
	if (ev.changed || len(ev.undelivered) > 0) && s.OnChange != nil {
		s.OnChange()
	}
}

func validateAsk(in AskInput) error {
	if strings.TrimSpace(in.TabID) == "" {
		return errors.New("vault: an ask needs the tab id, run it from a lemongrass tab")
	}
	if !filepath.IsAbs(in.Cwd) {
		return fmt.Errorf("vault: an ask needs an absolute working directory, got %q", in.Cwd)
	}
	if err := capText("reason", in.Reason, MaxAskReason); err != nil {
		return err
	}
	needsReason := func() error {
		if strings.TrimSpace(in.Reason) == "" {
			return fmt.Errorf("vault: a %s ask needs a reason", in.Type)
		}
		return nil
	}
	if in.Type != AskCommand && len(in.AccountEnv) > 0 {
		return errors.New("vault: only a command ask carries account variables")
	}
	switch in.Type {
	case AskDB:
		if err := needsReason(); err != nil {
			return err
		}
		if len(in.Tables) == 0 {
			return errors.New("vault: a db ask needs at least one table")
		}
		if len(in.Tables) > MaxAskTables {
			return fmt.Errorf("vault: a db ask names at most %d tables", MaxAskTables)
		}
		for _, table := range in.Tables {
			if strings.TrimSpace(table) == "" || len(table) > MaxAskTableName || strings.ContainsAny(table, " \t\r\n") {
				return fmt.Errorf("vault: %q is not a table name", table)
			}
		}
		if err := CheckAskOperations(in.Operations); err != nil {
			return err
		}
		if in.Message != "" || in.Command != "" || len(in.Methods) > 0 {
			return errors.New("vault: a db ask takes tables, operations and a reason only")
		}
	case AskRester:
		if strings.TrimSpace(in.Message) == "" {
			return errors.New("vault: a rester ask needs a message")
		}
		if err := capText("message", in.Message, MaxAskMessage); err != nil {
			return err
		}
		if err := CheckAskMethods(in.Methods); err != nil {
			return err
		}
		if len(in.Tables) > 0 || len(in.Operations) > 0 || in.Command != "" {
			return errors.New("vault: a rester ask takes a message, methods and an optional reason only")
		}
	case AskCommand:
		if err := needsReason(); err != nil && in.Rule == "" {
			return err
		}
		if strings.TrimSpace(in.Command) == "" {
			return errors.New("vault: a command ask needs a command")
		}
		if err := capText("command", in.Command, MaxAskCommand); err != nil {
			return err
		}
		if len(in.Tables) > 0 || len(in.Operations) > 0 || in.Message != "" || len(in.Methods) > 0 {
			return errors.New("vault: a command ask takes a command and a reason only")
		}
		if err := checkAccountEnv(in.AccountEnv); err != nil {
			return err
		}
	default:
		return fmt.Errorf("vault: unknown ask type %q, want db, rester or command", in.Type)
	}
	return nil
}

// CheckAskOperations accepts a db ask's declared operations: at least one, each known, none repeated.
func CheckAskOperations(ops []string) error {
	return checkDeclared("operation", ops, AskDBOperations)
}

// CheckAskMethods accepts a rester ask's declared methods: at least one, each known and upper case, none repeated.
func CheckAskMethods(methods []string) error {
	return checkDeclared("method", methods, AskHTTPMethods)
}

func checkDeclared(kind string, got, known []string) error {
	if len(got) == 0 {
		return fmt.Errorf("vault: the ask needs at least one %s, from %s", kind, strings.Join(known, ", "))
	}
	seen := map[string]bool{}
	for _, v := range got {
		ok := false
		for _, k := range known {
			ok = ok || v == k
		}
		if !ok {
			return fmt.Errorf("vault: %q is not a known %s, want one of %s", v, kind, strings.Join(known, ", "))
		}
		if seen[v] {
			return fmt.Errorf("vault: %s %q is listed twice", kind, v)
		}
		seen[v] = true
	}
	return nil
}

// checkAccountEnv accepts only the known account variables, each an absolute path.
func checkAccountEnv(env map[string]string) error {
	for k, v := range env {
		if !slices.Contains(AskAccountEnvKeys, k) {
			return fmt.Errorf("vault: %q is not an account variable, want one of %s", k, strings.Join(AskAccountEnvKeys, ", "))
		}
		if err := capText(k, v, 4096); err != nil {
			return err
		}
		if !filepath.IsAbs(v) {
			return fmt.Errorf("vault: %s must be an absolute path", k)
		}
	}
	return nil
}

func capText(field, text string, max int) error {
	if len(text) > max {
		return fmt.Errorf("vault: the %s is %d bytes and the cap is %d", field, len(text), max)
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("vault: the %s is not valid UTF-8", field)
	}
	return nil
}

func newAskID() (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("vault: generating an ask id: %w", err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

// Create files an ask. A hook-filed ask whose tab already has the same command open returns that ask, so a model repeating a refused call files it once.
func (s *AskStore) Create(in AskInput) (Ask, error) {
	if err := validateAsk(in); err != nil {
		return Ask{}, err
	}
	s.mu.Lock()
	ev := s.sweepLocked()
	ask, err := s.createLocked(in)
	if err == nil {
		ev.changed = true
	}
	s.mu.Unlock()
	s.fire(ev)
	return ask, err
}

func (s *AskStore) createLocked(in AskInput) (Ask, error) {
	now := s.now()
	open := 0
	for _, r := range s.asks {
		if r.TabID != in.TabID || r.Final() {
			continue
		}
		open++
		if in.Rule != "" && r.Type == AskCommand && r.Command == in.Command {
			return r.Ask, nil
		}
	}
	if open >= askOpenMax {
		return Ask{}, fmt.Errorf("vault: this tab already has %d asks open, wait for an answer before asking again", open)
	}
	recent := s.filings[in.TabID][:0]
	for _, at := range s.filings[in.TabID] {
		if now.Sub(at) < askRateWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) >= askRateMax {
		s.filings[in.TabID] = recent
		return Ask{}, fmt.Errorf("vault: this tab filed %d asks in the last %s, wait before asking again", len(recent), askRateWindow)
	}
	if len(s.asks) >= MaxAsks {
		return Ask{}, errors.New("vault: too many asks are held, answer or decline some first")
	}
	id, err := newAskID()
	if err != nil {
		return Ask{}, err
	}
	for s.asks[id] != nil {
		if id, err = newAskID(); err != nil {
			return Ask{}, err
		}
	}
	s.filings[in.TabID] = append(recent, now)
	r := &askRecord{
		Ask: Ask{
			ID: id, Type: in.Type, TabID: in.TabID, Cwd: filepath.Clean(in.Cwd), CreatedAt: now, State: AskWaiting,
			Reason: in.Reason, Tables: append([]string(nil), in.Tables...), Operations: append([]string(nil), in.Operations...),
			Message: in.Message, Methods: append([]string(nil), in.Methods...), Command: in.Command,
			Rule: in.Rule, Path: in.Path, AccountEnv: maps.Clone(in.AccountEnv),
		},
		done: make(chan struct{}),
	}
	s.asks[id] = r
	return r.snapshot(), nil
}

func (r *askRecord) snapshot() Ask {
	a := r.Ask
	a.Blocking = r.waiters > 0 && !r.Final()
	a.Tables = append([]string(nil), r.Tables...)
	a.Operations = append([]string(nil), r.Operations...)
	a.Methods = append([]string(nil), r.Methods...)
	a.RemovedTables = append([]RemovedTable(nil), r.RemovedTables...)
	a.AccountEnv = maps.Clone(r.AccountEnv)
	if r.Granted != nil {
		g := AskGrant{
			Tables:     append([]string(nil), r.Granted.Tables...),
			Operations: append([]string(nil), r.Granted.Operations...),
			Methods:    append([]string(nil), r.Granted.Methods...),
		}
		a.Granted = &g
	}
	if r.Result != nil {
		result := *r.Result
		a.Result = &result
	}
	return a
}

// finishLocked gives r its outcome and wakes its waiters. With no waiter attached the outcome is reported as undelivered.
func (s *AskStore) finishLocked(r *askRecord, ev *askEvents) {
	close(r.done)
	ev.changed = true
	if r.waiters == 0 && !r.delivered && r.State != AskExpired {
		ev.undelivered = append(ev.undelivered, r.snapshot())
	}
}

// sweepLocked expires asks past their lifetime. A waiting ask is marked expired first so a waiter sees why it ended; a running command is kept until it finishes.
func (s *AskStore) sweepLocked() askEvents {
	var ev askEvents
	now := s.now()
	for id, r := range s.asks {
		if now.Sub(r.CreatedAt) < AskLifetime || r.State == AskRunning {
			continue
		}
		if r.State == AskWaiting {
			r.State = AskExpired
			s.finishLocked(r, &ev)
		}
		delete(s.asks, id)
		ev.changed = true
	}
	for tab, times := range s.filings {
		if len(times) == 0 || now.Sub(times[len(times)-1]) >= askRateWindow {
			delete(s.filings, tab)
		}
	}
	return ev
}

// Sweep expires old asks; the daemon runs it on a ticker so expiry does not wait for the next op.
func (s *AskStore) Sweep() {
	s.mu.Lock()
	ev := s.sweepLocked()
	s.mu.Unlock()
	s.fire(ev)
}

func (s *AskStore) lookupLocked(id string) (*askRecord, error) {
	r := s.asks[id]
	if r == nil {
		return nil, ErrNoAsk
	}
	return r, nil
}

// Wait blocks until the ask has its outcome or timeout passes, and returns it as it then stands. tabID must be the ask's own tab. An outcome returned here counts as delivered only once Ack confirms it, or a nudge follows after AskAckGrace.
func (s *AskStore) Wait(id, tabID string, timeout time.Duration) (Ask, error) {
	if timeout <= 0 || timeout > AskMaxWait {
		timeout = AskMaxWait
	}
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	if err == nil && r.TabID != tabID {
		err = ErrNoAsk
	}
	if err != nil {
		s.mu.Unlock()
		s.fire(ev)
		return Ask{}, err
	}
	if r.Final() {
		a := r.snapshot()
		s.awaitAckLocked(r)
		s.mu.Unlock()
		s.fire(ev)
		return a, nil
	}
	r.waiters++
	done := r.done
	ev.changed = true
	s.mu.Unlock()
	s.fire(ev)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}

	s.mu.Lock()
	r.waiters--
	a := r.snapshot()
	if r.Final() {
		s.awaitAckLocked(r)
	}
	s.mu.Unlock()
	s.fire(askEvents{changed: true})
	return a, nil
}

// awaitAckLocked starts the grace period after which an unacknowledged outcome goes to the tab as a nudge.
func (s *AskStore) awaitAckLocked(r *askRecord) {
	if r.delivered || r.State == AskExpired {
		return
	}
	id := r.ID
	time.AfterFunc(s.grace, func() { s.Delivered(id, false) })
}

// Ack records that the model has an ask's outcome. tabID must be the ask's own tab.
func (s *AskStore) Ack(id, tabID string) error {
	s.mu.Lock()
	r := s.asks[id]
	if r == nil || r.TabID != tabID {
		s.mu.Unlock()
		return ErrNoAsk
	}
	if r.Final() {
		r.delivered = true
	}
	s.mu.Unlock()
	return nil
}

// Delivered records whether a final ask's outcome reached the model. An outcome that did not arrive, while no other waiter is attached to receive it, goes to the tab as a nudge instead.
func (s *AskStore) Delivered(id string, ok bool) {
	s.mu.Lock()
	r := s.asks[id]
	var ev askEvents
	if r != nil && r.Final() && !r.delivered {
		if ok {
			r.delivered = true
		} else if r.waiters == 0 && r.State != AskExpired {
			r.delivered = true
			ev.undelivered = append(ev.undelivered, r.snapshot())
		}
	}
	s.mu.Unlock()
	s.fire(ev)
}

// Result returns an ask to the tab that filed it.
func (s *AskStore) Result(id, tabID string) (Ask, error) {
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	var a Ask
	if err == nil && r.TabID != tabID {
		err = ErrNoAsk
	}
	if err == nil {
		a = r.snapshot()
	}
	s.mu.Unlock()
	s.fire(ev)
	return a, err
}

// Get returns any ask, for the app.
func (s *AskStore) Get(id string) (Ask, error) {
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	var a Ask
	if err == nil {
		a = r.snapshot()
	}
	s.mu.Unlock()
	s.fire(ev)
	return a, err
}

// List returns the asks waiting for a decision and the commands still running, oldest first.
func (s *AskStore) List() []Ask {
	s.mu.Lock()
	ev := s.sweepLocked()
	var out []Ask
	for _, r := range s.asks {
		if r.State == AskWaiting || r.State == AskRunning {
			out = append(out, r.snapshot())
		}
	}
	s.mu.Unlock()
	s.fire(ev)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Count is the number of asks waiting for a decision, how many of them a model is still waiting on, and the newest one's id. It carries no ask content, so it is served without the root secret.
func (s *AskStore) Count() (waiting, blocking int, newestID string) {
	s.mu.Lock()
	ev := s.sweepLocked()
	var newest *askRecord
	for _, r := range s.asks {
		if r.State != AskWaiting {
			continue
		}
		waiting++
		if r.waiters > 0 {
			blocking++
		}
		if newest == nil || r.CreatedAt.After(newest.CreatedAt) || (r.CreatedAt.Equal(newest.CreatedAt) && r.ID > newest.ID) {
			newest = r
		}
	}
	if newest != nil {
		newestID = newest.ID
	}
	s.mu.Unlock()
	s.fire(ev)
	return waiting, blocking, newestID
}

// Decline ends a waiting ask with the human's note and, for a db ask, the tables the human removed and why.
func (s *AskStore) Decline(id, note string, removed []RemovedTable) error {
	if err := capText("note", note, MaxAskReason); err != nil {
		return err
	}
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	if err == nil && r.State != AskWaiting {
		err = ErrAskNotWaiting
	}
	if err == nil {
		r.State = AskDeclined
		r.Note = note
		r.RemovedTables = append([]RemovedTable(nil), removed...)
		s.finishLocked(r, &ev)
	}
	s.mu.Unlock()
	s.fire(ev)
	return err
}

// CheckChannelAsk confirms id is a waiting channel ask of the given type, before a channel is created for it.
func (s *AskStore) CheckChannelAsk(id, askType string) (Ask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.lookupLocked(id)
	if err != nil {
		return Ask{}, err
	}
	if r.Type != askType {
		return Ask{}, fmt.Errorf("vault: ask %s is a %s ask, not a %s ask", id, r.Type, askType)
	}
	if r.State != AskWaiting {
		return Ask{}, ErrAskNotWaiting
	}
	return r.snapshot(), nil
}

// LinkChannel records that channel was created for the waiting channel ask id, with what it grants and the requested tables it leaves out.
func (s *AskStore) LinkChannel(id, askType string, channel ChannelID, granted AskGrant, removed []RemovedTable) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.lookupLocked(id)
	if err != nil {
		return err
	}
	if r.Type != askType {
		return fmt.Errorf("vault: ask %s is a %s ask, not a %s ask", id, r.Type, askType)
	}
	if r.State != AskWaiting {
		return ErrAskNotWaiting
	}
	r.channel = channel
	r.Granted = &granted
	r.RemovedTables = append([]RemovedTable(nil), removed...)
	return nil
}

// LinkedChannel is the channel created for a channel ask, empty when none was.
func (s *AskStore) LinkedChannel(id string) (ChannelID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.lookupLocked(id)
	if err != nil {
		return "", err
	}
	return r.channel, nil
}

// AnswerChannel answers a channel ask with the short id the agent minted for the channel created for it.
func (s *AskStore) AnswerChannel(id, shortID string) error {
	shortID = strings.TrimSpace(shortID)
	if shortID == "" || len(shortID) > MaxAskShortID || strings.ContainsAny(shortID, " \t\r\n") {
		return fmt.Errorf("vault: %q is not a short channel id", shortID)
	}
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	switch {
	case err != nil:
	case r.Type == AskCommand:
		err = errors.New("vault: a command ask is answered by executing it, not with a channel")
	case r.State != AskWaiting:
		err = ErrAskNotWaiting
	case r.channel == "":
		err = fmt.Errorf("vault: no channel was created for ask %s, create one with its ask id first", id)
	default:
		r.State = AskAnswered
		r.ShortID = shortID
		s.finishLocked(r, &ev)
	}
	s.mu.Unlock()
	s.fire(ev)
	return err
}

// Start moves a waiting command ask to running and returns it.
func (s *AskStore) Start(id string) (Ask, error) {
	s.mu.Lock()
	ev := s.sweepLocked()
	r, err := s.lookupLocked(id)
	switch {
	case err != nil:
	case r.Type != AskCommand:
		err = fmt.Errorf("vault: ask %s is a %s ask, only a command ask is executed", id, r.Type)
	case r.State != AskWaiting:
		err = ErrAskNotWaiting
	default:
		r.State = AskRunning
		r.Result = &AskResult{StartedAt: s.now()}
		ev.changed = true
	}
	var a Ask
	if err == nil {
		a = r.snapshot()
	}
	s.mu.Unlock()
	s.fire(ev)
	return a, err
}

// Abort puts a command ask back to waiting when its command could not be started at all.
func (s *AskStore) Abort(id string) {
	s.mu.Lock()
	r := s.asks[id]
	if r != nil && r.State == AskRunning {
		r.State = AskWaiting
		r.Result = nil
	}
	s.mu.Unlock()
	s.fire(askEvents{changed: true})
}

// Finish answers a running command ask with its result.
func (s *AskStore) Finish(id string, result AskResult) {
	s.mu.Lock()
	var ev askEvents
	r := s.asks[id]
	if r != nil && r.State == AskRunning {
		if r.Result != nil {
			result.StartedAt = r.Result.StartedAt
		}
		if result.EndedAt.IsZero() {
			result.EndedAt = s.now()
		}
		r.Result = &result
		r.State = AskAnswered
		s.finishLocked(r, &ev)
	}
	s.mu.Unlock()
	s.fire(ev)
}
