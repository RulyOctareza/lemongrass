// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "lemongrass-vault"
	keyringUser    = "root-secret"
)

func SaveRootSecret(secret string) error {
	if err := keyring.Set(keyringService, keyringUser, secret); err != nil {
		return fmt.Errorf("vault: saving root secret to OS keychain: %w", err)
	}
	return nil
}

func LoadRootSecret() (string, error) {
	secret, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		return "", fmt.Errorf("vault: loading root secret from OS keychain: %w", err)
	}
	return secret, nil
}

func DeleteRootSecret() error {
	if err := keyring.Delete(keyringService, keyringUser); err != nil {
		return fmt.Errorf("vault: deleting root secret from OS keychain: %w", err)
	}
	return nil
}
