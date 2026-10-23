package vault

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrChannelExpired  = errors.New("vault: this channel has expired. Ask the channel owner to reactivate it or share a fresh channel id.")
	ErrChannelInactive = errors.New("vault: this channel is not active")
)

// OpenChannel returns an active db channel's metadata and its decrypted connection string. The caller must Zero the returned bytes. An expired channel has its cached key released and fails with ErrChannelExpired.
func (s *Service) OpenChannel(id ChannelID) (Channel, []byte, error) {
	c, err := s.loadMeta(id)
	if err != nil {
		return Channel{}, nil, err
	}
	if c.Expired(time.Now()) {
		s.forgetKey(id)
		return Channel{}, nil, ErrChannelExpired
	}
	secret, err := s.channelSecret(id)
	if err != nil {
		return Channel{}, nil, err
	}
	return c, secret, nil
}

// OpenHTTPChannel returns an active HTTP channel's metadata and its decrypted resolved rester. An expired channel has its cached key released and fails with ErrChannelExpired.
func (s *Service) OpenHTTPChannel(id ChannelID) (HTTPChannel, ResolvedRester, error) {
	c, err := s.loadHTTPMeta(id)
	if err != nil {
		return HTTPChannel{}, ResolvedRester{}, err
	}
	if c.Expired(time.Now()) {
		s.forgetKey(id)
		return HTTPChannel{}, ResolvedRester{}, ErrChannelExpired
	}
	resolved, err := s.ChannelResolved(id)
	if err != nil {
		return HTTPChannel{}, ResolvedRester{}, err
	}
	return c, resolved, nil
}

// ChannelResolved decrypts channel id's own copy of its rester, failing when the channel is not active.
func (s *Service) ChannelResolved(id ChannelID) (ResolvedRester, error) {
	wrapped, err := s.channelSecret(id)
	if err != nil {
		return ResolvedRester{}, err
	}
	defer Zero(wrapped)

	resolved, err := decodeResolved(wrapped)
	if err != nil {
		return ResolvedRester{}, fmt.Errorf("vault: decoding this channel's rester: %w", err)
	}
	return resolved, nil
}

// ResolvedWeb is what a web channel grants the browser: its environment's origin and login
// steps and the effective users that can log in on the web side. BaseURL, TokenPath and
// TokenPlacement come from the api side and are empty without one; they tell the browser which
// requests carry a token to redact.
type ResolvedWeb struct {
	Environment    string
	Origin         string
	BaseURL        string
	Login          []LoginStep
	Users          []ResterUser
	TokenPath      string
	TokenPlacement TokenPlacement
}

// ErrNotWebChannel is returned where a web channel is required and the channel is an api one.
var ErrNotWebChannel = errors.New("vault: this channel is not a web channel")

// OpenWebChannel returns an active web channel's metadata and its resolved web side, failing for an api channel, an inactive one or an expired one.
func (s *Service) OpenWebChannel(id ChannelID) (HTTPChannel, ResolvedWeb, error) {
	c, resolved, err := s.OpenHTTPChannel(id)
	if err != nil {
		return HTTPChannel{}, ResolvedWeb{}, err
	}
	if c.Type != TypeWeb {
		return HTTPChannel{}, ResolvedWeb{}, ErrNotWebChannel
	}
	return c, ResolvedWeb{
		Environment:    resolved.Environment,
		Origin:         resolved.Origin,
		BaseURL:        resolved.BaseURL,
		Login:          resolved.Login,
		Users:          resolved.Users,
		TokenPath:      resolved.TokenPath,
		TokenPlacement: resolved.TokenPlacement,
	}, nil
}

// WithConnection decrypts dbName's stored connection string under rootSecret, passes it to fn, and zeroes it before returning.
func (s *Service) WithConnection(rootSecret, dbName string, fn func(connString []byte) error) error {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)

	connString, err := s.creds.Get(dbName, rootKey)
	if err != nil {
		return fmt.Errorf("vault: reading credential for %s: %w", dbName, err)
	}
	defer Zero(connString)
	return fn(connString)
}

// WithConnections decrypts every stored connection string under rootSecret, deriving the root
// key once, passes each to fn, and zeroes each after fn returns.
func (s *Service) WithConnections(rootSecret string, fn func(dbName string, connString []byte) error) error {
	names, err := s.ListConnections()
	if err != nil {
		return err
	}
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)

	for _, name := range names {
		connString, err := s.creds.Get(name, rootKey)
		if err != nil {
			return fmt.Errorf("vault: reading credential for %s: %w", name, err)
		}
		err = fn(name, connString)
		Zero(connString)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnInvalidate registers fn to run whenever a channel's cached state must be dropped: revocation, expiry, or a rewrap after an edit.
func (s *Service) OnInvalidate(fn func(ChannelID)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onInvalid = append(s.onInvalid, fn)
}

func (s *Service) invalidate(id ChannelID) {
	s.mu.Lock()
	hooks := append([](func(ChannelID)){}, s.onInvalid...)
	s.mu.Unlock()
	for _, fn := range hooks {
		fn(id)
	}
}

func (s *Service) channelSecret(id ChannelID) ([]byte, error) {
	s.mu.Lock()
	channelKey, ok := s.activeKeys[id]
	s.mu.Unlock()
	if !ok {
		return nil, ErrChannelInactive
	}
	return s.channels.Get(string(id), channelKey)
}
