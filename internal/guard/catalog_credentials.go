// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var credentialRules = []rule{
	{ID: "credential-read", Category: catCredentials, Description: "printing a token, password or credential", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"gh"}, Subs: []string{"auth token"}},
		{Tools: []string{"gh"}, Subs: []string{"auth status"}, Flags: []string{"-t", "--show-token"}},
		{Tools: []string{"glab"}, Subs: []string{"auth status"}, Flags: []string{"-t", "--show-token"}},
		{Tools: []string{"gcloud"}, Subs: []string{"auth print-access-token", "auth print-identity-token", "auth application-default print-access-token", "secrets versions access"}},
		{Tools: []string{"az"}, Subs: []string{"account get-access-token", "keyvault secret show", "keyvault secret download", "storage account keys list", "acr credential show", "aks get-credentials"}},
		{Tools: []string{"aws"}, Subs: []string{"configure get", "configure export-credentials", "ecr get-login-password", "ecr-public get-login-password", "sts get-session-token", "sts assume-role", "sts assume-role-with-web-identity", "sts get-federation-token", "secretsmanager get-secret-value", "ssm get-parameter", "ssm get-parameters", "ssm get-parameters-by-path", "codeartifact get-authorization-token", "eks get-token"}},
		{Tools: []string{"kubectl", "oc"}, Subs: []string{"config view"}, Flags: []string{"--raw", "--flatten"}},
		{Tools: []string{"kubectl", "oc"}, Subs: []string{"create token"}},
		{Tools: []string{"oc"}, Subs: []string{"whoami"}, Flags: []string{"-t", "--show-token"}},
		{ToolPrefixes: []string{"docker-credential-", "git-credential-"}},
		{Tools: []string{"git"}, Subs: []string{"credential fill", "credential-cache", "credential-store"}},
		{Tools: []string{"npm"}, Subs: []string{"token"}},
		{Tools: []string{"heroku"}, Subs: []string{"auth:token", "authorizations:create"}},
		{Tools: []string{"fly", "flyctl"}, Subs: []string{"auth token", "tokens create"}},
		{Tools: []string{"doctl"}, Subs: []string{"registry login", "kubernetes cluster kubeconfig show"}},
		{Tools: []string{"op", "bw", "pass", "gopass", "lpass", "rbw", "keepassxc-cli", "sops"}},
		{Tools: []string{"vault"}, Subs: []string{"read", "kv get", "token", "print token"}},
		{Tools: []string{"security"}, Subs: []string{"find-generic-password", "find-internet-password", "dump-keychain", "export"}},
		{Tools: []string{"gpg", "gpg2"}, Flags: []string{"--export-secret-keys", "--export-secret-subkeys", "-d", "--decrypt"}},
	}},
	{ID: "registry-login", Category: catCredentials, Description: "logging in to a registry, cloud or forge, or storing a token", Mode: ModeDeny, Forms: []form{
		{Tools: append(append([]string{"buildah", "skopeo", "oras", "crane", "regctl"}, containerEngines...), composeTools...), Subs: []string{"login", "logout", "auth login", "registry login"}},
		{Tools: []string{"helm"}, Subs: []string{"registry login", "repo add"}, Flags: []string{"--password", "--password-stdin", "--username", "-u", "--pass-credentials"}},
		{Tools: []string{"helm"}, Subs: []string{"registry login"}},
		{Tools: []string{"npm", "pnpm", "yarn"}, Subs: []string{"login", "adduser", "add-user", "logout", "npm login"}},
		{Tools: []string{"npm", "pnpm", "yarn"}, Subs: []string{"config set"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			for _, arg := range call.Args {
				lower := strings.ToLower(arg)
				if strings.Contains(lower, "_authtoken") || strings.Contains(lower, "_auth") || strings.Contains(lower, "_password") || strings.Contains(lower, "npmauthtoken") {
					return "Matched: " + call.Name + " config set storing a registry credential.", true
				}
			}
			return "", false
		}},
		{Tools: []string{"cargo"}, Subs: []string{"login", "logout"}},
		{Tools: []string{"gem"}, Subs: []string{"signin", "signout"}},
		{Tools: []string{"gh", "glab"}, Subs: []string{"auth login", "auth logout", "auth refresh", "auth setup-git", "auth switch"}},
		{Tools: []string{"heroku", "fly", "flyctl", "vercel", "netlify", "wrangler", "firebase", "railway", "doctl", "pulumi", "snapcraft", "vsce", "ovsx", "ngrok", "tailscale", "dotnet", "huggingface-cli", "hf", "poetry"}, Subs: []string{"login", "auth login", "auth init", "logout", "auth logout", "auth:login", "config:add-authtoken", "config add-authtoken", "nuget add source", "config http-basic", "auth token"}},
		{Tools: []string{"gcloud"}, Subs: []string{"auth login", "auth activate-service-account", "auth application-default login", "auth configure-docker", "auth revoke"}},
		{Tools: []string{"az"}, Subs: []string{"login", "logout", "acr login"}},
		{Tools: []string{"aws"}, Subs: []string{"configure set", "configure import", "sso login", "sso logout", "ecr get-login", "codeartifact login"}},
		{Tools: []string{"aws"}, Subs: []string{"configure"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(len(positionals(call)) == 1)
		}},
		{Tools: []string{"poetry"}, Subs: []string{"config"}, ArgPrefixes: []string{"pypi-token.", "http-basic."}},
		{Tools: []string{"pip", "pip3"}, Subs: []string{"config set"}, ArgPrefixes: []string{"global.index-url", "global.extra-index-url"}},
	}},
}
