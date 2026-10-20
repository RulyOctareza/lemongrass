package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// UpdateRester replaces the stored rester name with r, then re-wraps r for every HTTP channel
// minted from that rester, active or not, each resolved for its own environment and type, and
// invalidates their cached state so the next call logs in with the new config. The existing
// rester is decrypted first, which fails on a wrong passphrase or an unknown name before anything
// is written. An edit that removes an environment or a side some channel uses is refused,
// naming the channels. A channel that cannot be rewritten is named in the returned error; saving
// the same edit again retries it.
func (s *Service) UpdateRester(rootSecret, name string, r Rester) error {
	if _, err := s.GetRester(rootSecret, name); err != nil {
		return err
	}
	if err := r.validateNamed(name); err != nil {
		return err
	}
	// The channel check and the write are one step against a concurrent CreateHTTPChannel, which takes the same lock.
	s.resterMu.Lock()
	locked := true
	defer func() {
		if locked {
			s.resterMu.Unlock()
		}
	}()
	channels, err := s.ListHTTPChannels()
	if err != nil {
		return err
	}
	var mine []HTTPChannel
	var blocked []string
	for _, c := range channels {
		if c.Rester != name {
			continue
		}
		if _, err := r.resolve(c.Environment, c.Type); err != nil {
			blocked = append(blocked, fmt.Sprintf("%s (%s, %s channel)", c.Name, c.Environment, c.Type))
			continue
		}
		mine = append(mine, c)
	}
	if len(blocked) > 0 {
		return errors.New("vault: rester " + name + " not saved, it would remove an environment or side these channels use: " + strings.Join(blocked, "; "))
	}

	err = s.putRester(rootSecret, name, r)
	s.resterMu.Unlock()
	locked = false
	if err != nil {
		return err
	}

	var failed []string
	for _, c := range mine {
		if err := s.rewrapHTTPChannel(rootSecret, c, r); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", c.ID, err))
			continue
		}
		s.invalidate(c.ID)
	}
	if len(failed) > 0 {
		return errors.New("vault: rester " + name + " saved, but these channels still hold the old config, save again to retry: " + strings.Join(failed, "; "))
	}
	return nil
}

func (s *Service) rewrapHTTPChannel(rootSecret string, c HTTPChannel, r Rester) error {
	resolved, err := r.resolve(c.Environment, c.Type)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("encoding the rester: %w", err)
	}
	defer Zero(plain)
	key, err := DeriveKey(rootSecret, c.Salt)
	if err != nil {
		return err
	}
	defer Zero(key)
	return s.channels.Put(string(c.ID), key, plain)
}
