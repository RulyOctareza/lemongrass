// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
)

// Validate checks a rester before it is stored: at least one side and one environment, unique
// environment names, valid addresses and headers, well-formed login steps, and users whose
// assignments, overrides and credentials agree. A rester with no users is allowed, though no
// channel can be made on it.
func (r Rester) Validate() error {
	if r.API == nil && r.Web == nil && r.App == nil {
		return fmt.Errorf("vault: a rester needs an api side, a web side, an app side or a combination")
	}
	if len(r.Environments) == 0 {
		return fmt.Errorf("vault: a rester needs at least one environment")
	}
	if err := r.checkEnvironments(); err != nil {
		return err
	}
	if r.Web != nil {
		if err := checkLoginSteps(r.Web.Login); err != nil {
			return err
		}
	}
	if r.App != nil {
		if err := checkAppLogin(*r.App); err != nil {
			return err
		}
	}
	return r.checkUsers()
}

// validateNamed is Validate with the rester's name in the error, so a human saving an old record that no
// longer passes a check knows which one to fix.
func (r Rester) validateNamed(name string) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("vault: rester %q: %s", name, strings.TrimPrefix(err.Error(), "vault: "))
	}
	return nil
}

func (r Rester) checkEnvironments() error {
	var placement TokenPlacement
	if r.API != nil {
		placement = r.API.TokenPlacement
		if err := checkHeaderList(r.API.Headers, placement, "the api side"); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(r.Environments))
	for _, e := range r.Environments {
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("vault: an environment has no name")
		}
		if seen[e.Name] {
			return fmt.Errorf("vault: environment %q is listed more than once", e.Name)
		}
		seen[e.Name] = true
		if e.BaseURL != "" {
			if err := checkAddress(e.BaseURL, false); err != nil {
				return fmt.Errorf("vault: environment %q base URL: %w", e.Name, err)
			}
		}
		if e.Origin != "" {
			if err := checkAddress(e.Origin, true); err != nil {
				return fmt.Errorf("vault: environment %q origin: %w", e.Name, err)
			}
		}
		if err := checkHeaderList(e.Headers, placement, fmt.Sprintf("environment %q", e.Name)); err != nil {
			return err
		}
	}
	return nil
}

// checkAddress wants an http or https URL with a host, and for an origin nothing after the host.
func checkAddress(s string, origin bool) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%q is not an http or https address", s)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%q must not carry credentials, a query or a fragment", s)
	}
	if origin && u.Path != "" && u.Path != "/" {
		return fmt.Errorf("%q is an origin, so it has no path", s)
	}
	return nil
}

func checkLoginSteps(steps []LoginStep) error {
	for i, s := range steps {
		n := i + 1
		var missing string
		switch s.Action {
		case StepGoto:
			if s.URL == "" {
				missing = "a URL"
			}
		case StepFill:
			if s.Selector == "" || s.Field == "" {
				missing = "a selector and a field"
			}
		case StepClick:
			if s.Selector == "" {
				missing = "a selector"
			}
		case StepWait:
			if s.Selector == "" && s.URL == "" {
				missing = "a selector or a URL pattern"
			}
		default:
			return fmt.Errorf("vault: login step %d has an unknown action %q, want goto, fill, click or wait", n, s.Action)
		}
		if missing != "" {
			return fmt.Errorf("vault: login step %d (%s) needs %s", n, s.Action, missing)
		}
		if s.TimeoutSeconds < 0 {
			return fmt.Errorf("vault: login step %d has a negative timeout", n)
		}
	}
	return nil
}

// checkAppLogin wants a package name and steps that each carry what their action needs: a
// locator (Text or ID) for tap, fill and wait, plus a field for fill, and a key for key.
func checkAppLogin(a ResterApp) error {
	if strings.TrimSpace(a.Package) == "" {
		return fmt.Errorf("vault: the app side needs a package name")
	}
	for i, s := range a.Login {
		n := i + 1
		var missing string
		switch s.Action {
		case AppStepLaunch:
		case AppStepTap, AppStepWait:
			if s.Text == "" && s.ID == "" {
				missing = "a text or an id"
			}
		case AppStepFill:
			if (s.Text == "" && s.ID == "") || s.Field == "" {
				missing = "a text or an id, and a field"
			}
		case AppStepKey:
			if s.Key == "" {
				missing = "a key"
			}
		default:
			return fmt.Errorf("vault: app login step %d has an unknown action %q, want launch, tap, fill, key or wait", n, s.Action)
		}
		if missing != "" {
			return fmt.Errorf("vault: app login step %d (%s) needs %s", n, s.Action, missing)
		}
		if s.TimeoutSeconds < 0 {
			return fmt.Errorf("vault: app login step %d has a negative timeout", n)
		}
	}
	return nil
}

func (r Rester) checkUsers() error {
	seen := make(map[string]string, len(r.Users))
	for _, u := range r.Users {
		h := UserHandle(u.Name)
		if h == "" {
			return fmt.Errorf("vault: a user has no name")
		}
		if prev, ok := seen[h]; ok {
			return fmt.Errorf("vault: users %q and %q both map to the handle %q", prev, u.Name, h)
		}
		seen[h] = u.Name
		if err := r.checkUser(u); err != nil {
			return err
		}
	}
	return nil
}

func (r Rester) checkUser(u ResterUser) error {
	for i, name := range u.Environments {
		if _, ok := r.environment(name); !ok {
			return fmt.Errorf("vault: user %q is assigned to environment %q, which does not exist", u.Name, name)
		}
		if slices.Contains(u.Environments[:i], name) {
			return fmt.Errorf("vault: user %q is assigned to environment %q more than once", u.Name, name)
		}
	}
	for name := range u.Overrides {
		if _, ok := r.environment(name); !ok {
			return fmt.Errorf("vault: user %q overrides environment %q, which does not exist", u.Name, name)
		}
		if !u.assignedTo(name) {
			return fmt.Errorf("vault: user %q overrides environment %q, which the user is not assigned to", u.Name, name)
		}
	}
	// A user is a token user when it carries a token anywhere, and then every environment must
	// give it one and none may also give it login fields.
	tokenUser := u.Token != ""
	for _, o := range u.Overrides {
		tokenUser = tokenUser || o.Token != ""
	}
	if tokenUser && len(u.Environments) == 0 && len(u.Fields) > 0 {
		return fmt.Errorf("vault: user %q has both a token and login fields, a user is one or the other", u.Name)
	}
	if err := checkLoginFields(u, ""); err != nil {
		return err
	}
	for _, name := range u.Environments {
		eff := u.Effective(name)
		if eff.IsBYOT() && len(eff.Fields) > 0 {
			return fmt.Errorf("vault: user %q has both a token and login fields in environment %q, a user is one or the other", u.Name, name)
		}
		if tokenUser && !eff.IsBYOT() {
			return fmt.Errorf("vault: user %q is a token user but has no token in environment %q", u.Name, name)
		}
		if err := checkLoginFields(eff, name); err != nil {
			return err
		}
	}
	return nil
}

// checkLoginFields refuses a field whose value does not fit its type, naming the user and, for an effective user, its environment.
func checkLoginFields(u ResterUser, env string) error {
	for key, value := range u.Fields {
		if _, err := loginFieldValue(key, value, u.FieldTypes[key]); err != nil {
			var fe *loginFieldError
			if !errors.As(err, &fe) {
				return err
			}
			where := ""
			if env != "" {
				where = fmt.Sprintf(" in environment %q", env)
			}
			return fmt.Errorf("vault: user %q%s: login field %q %s", u.Name, where, fe.Key, fe.Reason)
		}
	}
	return nil
}

// Validate checks a web scope: origins that are bare http or https origins, and exclusions with a method and a path pattern path.Match accepts.
func (w WebScope) Validate() error {
	for _, o := range w.Origins {
		if err := checkAddress(o, true); err != nil {
			return fmt.Errorf("vault: web scope origin: %w", err)
		}
	}
	for _, ex := range w.Exclusions {
		if ex.Method == "" || ex.PathPattern == "" {
			return fmt.Errorf("vault: a web scope exclusion needs a method and a path pattern")
		}
		if _, err := path.Match(ex.PathPattern, ""); err != nil {
			return fmt.Errorf("vault: web scope exclusion pattern %q: %w", ex.PathPattern, err)
		}
	}
	return nil
}
