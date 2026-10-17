package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/faizalv/lemongrass/internal/agentconf"
	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

var errTrustUsage = errors.New("usage: lgrassd trust <absolute-path> [--target primary|codex|<account>]")

func cmdTrust(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	codexConfig := filepath.Join(home, ".codex", "config.toml")
	if err := runTrust(claudeaccount.Accounts{Home: home}, codexConfig, args); err != nil {
		if errors.Is(err, errTrustUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fail(err)
	}
}

func runTrust(accounts claudeaccount.Accounts, codexConfig string, args []string) error {
	if len(args) != 1 && (len(args) != 3 || args[1] != "--target") {
		return errTrustUsage
	}
	if !filepath.IsAbs(args[0]) {
		return errTrustUsage
	}
	path := filepath.Clean(args[0])
	if len(args) == 1 {
		return errors.Join(accounts.TrustAll(path), agentconf.TrustCodexProject(codexConfig, path))
	}
	switch target := args[2]; target {
	case "primary":
		return accounts.Trust("", path)
	case "codex":
		return agentconf.TrustCodexProject(codexConfig, path)
	default:
		if err := claudeaccount.ValidName(target); err != nil {
			return err
		}
		names, err := accounts.Names()
		if err != nil {
			return err
		}
		if !slices.Contains(names, target) {
			return fmt.Errorf("lgrassd: no Claude account named %q", target)
		}
		return accounts.Trust(target, path)
	}
}
