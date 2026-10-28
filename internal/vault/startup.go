// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import "time"

// ExpireKeylessChannels sets ExpiresAt to now on every unexpired db and HTTP channel that has no
// cached key. A freshly started vault holds no key, so its channels are already unusable, and
// storing them as expired keeps the UI and every error from reporting a channel as open.
func (s *Service) ExpireKeylessChannels(now time.Time) error {
	channels, err := s.ListChannels()
	if err != nil {
		return err
	}
	for _, c := range channels {
		if c.Expired(now) || s.hasKey(c.ID) {
			continue
		}
		c.ExpiresAt = now
		if err := s.saveMeta(c); err != nil {
			return err
		}
	}

	httpChannels, err := s.ListHTTPChannels()
	if err != nil {
		return err
	}
	for _, c := range httpChannels {
		if c.Expired(now) || s.hasKey(c.ID) {
			continue
		}
		c.ExpiresAt = now
		if err := s.saveHTTPMeta(c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) hasKey(id ChannelID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.activeKeys[id]
	return ok
}
