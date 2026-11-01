// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

var packageRules = []rule{
	{ID: "package-publish", Category: catPackages, Description: "publishing or unpublishing a package", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"npm"}, Subs: []string{"publish", "unpublish", "deprecate"}},
	}},
	{ID: "package-publish-yarn", Category: catPackages, Description: "publishing a package", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"yarn"}, Subs: []string{"publish", "npm publish"}},
	}},
	{ID: "package-publish-pnpm", Category: catPackages, Description: "publishing a package", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"pnpm"}, Subs: []string{"publish"}},
	}},
	{ID: "cargo-publish", Category: catPackages, Description: "publishing a crate", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"cargo"}, Subs: []string{"publish", "yank", "owner"}},
	}},
	{ID: "twine", Category: catPackages, Description: "publishing a python package", Mode: ModeDeny, Forms: []form{{Tools: []string{"twine"}}}},

	{ID: "registry-publish", Category: catPackages, Description: "publishing to or changing a package registry", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"gem"}, Subs: []string{"push", "yank", "owner"}},
		{Tools: []string{"poetry", "uv", "pdm", "flit", "hatch", "bun", "deno", "jsr", "vsce", "ovsx"}, Subs: []string{"publish"}},
		{Tools: []string{"vsce", "ovsx"}, Subs: []string{"unpublish"}},
		{Tools: []string{"dotnet"}, Subs: []string{"nuget push", "nuget delete"}},
		{Tools: []string{"nuget"}, Subs: []string{"push", "delete"}},
		{Tools: []string{"mvn", "mvnw"}, Args: []string{"deploy", "deploy:deploy", "deploy:deploy-file", "release:perform"}},
		{Tools: []string{"gradle", "gradlew"}, ArgPrefixes: []string{"publish", ":publish"}},
		{Tools: []string{"mix"}, Subs: []string{"hex.publish", "hex.retire", "hex.owner"}},
		{Tools: []string{"dart", "flutter"}, Subs: []string{"pub publish", "pub uploader"}},
		{Tools: []string{"pod"}, Subs: []string{"trunk push", "trunk delete", "trunk deprecate", "trunk add-owner", "trunk remove-owner"}},
		{Tools: []string{"npm"}, Subs: []string{"dist-tag add", "dist-tag rm", "dist-tag remove", "owner add", "owner rm", "owner remove", "access", "team", "org set", "org rm", "star", "unstar", "hook"}},
		{Tools: []string{"snapcraft"}, Subs: []string{"upload", "release", "push", "close", "promote", "register"}},
		{Tools: []string{"goreleaser"}, Subs: []string{"release", "publish"}},
		{Tools: []string{"changeset"}, Subs: []string{"publish"}},
		{Tools: []string{"lerna"}, Subs: []string{"publish"}},
		{Tools: []string{"semantic-release", "release-it", "np"}},
		{Tools: []string{"opam"}, Subs: []string{"publish"}},
		{Tools: []string{"cabal"}, Subs: []string{"upload"}},
		{Tools: []string{"stack"}, Subs: []string{"upload"}},
		{Tools: []string{"luarocks"}, Subs: []string{"upload"}},
		{Tools: []string{"conan"}, Subs: []string{"upload", "remove"}},
		{Tools: []string{"helm"}, Subs: []string{"cm-push"}},
	}},
	{ID: "package-remove", Category: catPackages, Description: "removing global or system packages or wiping package caches", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"npm", "pnpm"}, Subs: []string{"uninstall", "rm", "remove", "un", "r", "unlink"}, Flags: []string{"-g", "--global"}},
		{Tools: []string{"yarn"}, Subs: []string{"global remove"}},
		{Tools: []string{"npm"}, Subs: []string{"cache clean"}},
		{Tools: []string{"pip", "pip3", "pipx"}, Subs: []string{"uninstall", "uninstall-all"}},
		{Tools: []string{"pip", "pip3"}, Subs: []string{"cache purge", "cache remove"}},
		{Tools: []string{"brew"}, Subs: []string{"uninstall", "remove", "rm", "cleanup", "autoremove", "untap"}},
		{Tools: []string{"flatpak"}, Subs: []string{"uninstall", "remove"}},
		{Tools: []string{"snap"}, Subs: []string{"remove"}},
		{Tools: []string{"apt", "apt-get"}, Subs: []string{"remove", "purge", "autoremove", "autopurge", "clean", "autoclean"}},
		{Tools: []string{"dnf", "yum", "microdnf"}, Subs: []string{"remove", "erase", "autoremove", "clean"}},
		{Tools: []string{"zypper"}, Subs: []string{"remove", "rm", "clean"}},
		{Tools: []string{"apk"}, Subs: []string{"del"}},
		{Tools: []string{"pacman", "yay", "paru"}, ArgPrefixes: []string{"-R", "-Sc"}},
		{Tools: []string{"rpm-ostree"}, Subs: []string{"uninstall", "reset", "cleanup"}},
		{Tools: []string{"cargo"}, Subs: []string{"uninstall"}},
		{Tools: []string{"go"}, Subs: []string{"clean"}, Args: []string{"-modcache", "-cache", "-fuzzcache"}},
		{Tools: []string{"uv"}, Subs: []string{"cache clean", "cache prune", "tool uninstall", "python uninstall"}},
		{Tools: []string{"pnpm"}, Subs: []string{"store prune"}},
		{Tools: []string{"yarn"}, Subs: []string{"cache clean"}},
		{Tools: []string{"conda", "mamba", "micromamba"}, Subs: []string{"remove", "uninstall", "env remove", "clean"}},
		{Tools: []string{"gem"}, Subs: []string{"uninstall", "cleanup"}},
		{Tools: []string{"rustup"}, Subs: []string{"uninstall", "toolchain uninstall", "toolchain remove", "self uninstall", "component remove", "target remove"}},
		{Tools: []string{"nix-collect-garbage"}},
		{Tools: []string{"nix"}, Subs: []string{"profile remove", "store gc", "store delete"}},
		{Tools: []string{"nix-env"}, Flags: []string{"-e", "--uninstall"}},
	}},
}
