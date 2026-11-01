// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var infrastructureRules = []rule{
	{ID: "terraform-destroy", Category: catInfrastructure, Description: "destroying infrastructure", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"terraform"}, Subs: []string{"destroy"}},
		{Tools: []string{"terraform"}, Subs: []string{"apply"}, Args: []string{"-destroy", "--destroy"}},
	}},
	{ID: "tofu-destroy", Category: catInfrastructure, Description: "destroying infrastructure", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"tofu"}, Subs: []string{"destroy"}},
		{Tools: []string{"tofu"}, Subs: []string{"apply"}, Args: []string{"-destroy", "--destroy"}},
	}},
	{ID: "cloud-delete", Category: catInfrastructure, Description: "deleting cloud resources", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"aws", "gcloud", "az", "doctl", "gsutil"}, Args: []string{"rm", "delete", "destroy", "terminate"}, ArgPrefixes: []string{"delete-", "terminate-", "remove-"}},
	}},

	{ID: "infra-destroy", Category: catInfrastructure, Description: "destroying a stack, app or infrastructure state", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"pulumi"}, Subs: []string{"destroy", "down", "stack rm", "state delete"}},
		{Tools: []string{"cdk"}, Subs: []string{"destroy"}},
		{Tools: []string{"sam"}, Subs: []string{"delete"}},
		{Tools: []string{"serverless", "sls"}, Subs: []string{"remove"}},
		{Tools: []string{"vagrant"}, Subs: []string{"destroy", "box remove"}},
		{Tools: []string{"fly", "flyctl"}, Subs: []string{"destroy", "apps destroy", "volumes destroy", "volumes delete", "machine destroy", "machine remove"}},
		{Tools: []string{"heroku"}, Subs: []string{"apps:destroy", "destroy", "addons:destroy", "pg:reset", "repo:purge_cache"}},
		{Tools: []string{"wrangler"}, Subs: []string{"delete", "d1 delete", "kv namespace delete", "r2 bucket delete"}},
		{Tools: []string{"vercel"}, Subs: []string{"remove", "rm"}},
		{Tools: []string{"netlify"}, Subs: []string{"sites:delete"}},
		{Tools: []string{"railway"}, Subs: []string{"down", "delete"}},
		{Tools: []string{"terraform", "tofu"}, Subs: []string{"state rm", "state push", "force-unlock", "workspace delete"}},
		{Tools: []string{"oci", "ibmcloud", "hcloud", "linode-cli", "vultr-cli", "scw", "exo"}, Args: []string{"delete", "destroy", "terminate", "rm", "remove"}},
	}},
	{ID: "infra-apply", Category: catInfrastructure, Description: "applying infrastructure changes", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"terraform", "tofu"}, Subs: []string{"apply", "import", "taint", "untaint", "state mv", "state replace-provider", "state import", "refresh"}},
		{Tools: []string{"pulumi"}, Subs: []string{"up", "update", "refresh", "import", "cancel", "stack init", "config set", "state move", "state unprotect"}},
		{Tools: []string{"cdk"}, Subs: []string{"deploy", "bootstrap", "import", "watch"}},
		{Tools: []string{"sam"}, Subs: []string{"deploy", "sync"}},
		{Tools: []string{"ansible-playbook", "ansible", "ansible-pull", "salt", "salt-ssh", "pyinfra"}},
		{Tools: []string{"packer"}, Subs: []string{"build"}},
		{Tools: []string{"terragrunt"}, Subs: []string{"apply", "run-all", "destroy", "import", "state"}},
		{Tools: []string{"crossplane"}, Subs: []string{"install", "xpkg push"}},
	}},
	{ID: "deploy", Category: catInfrastructure, Description: "deploying to a hosting platform", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"vercel"}, Match: vercelDeploys},
		{Tools: []string{"netlify"}, Subs: []string{"deploy", "env:set", "env:unset", "link", "sites:create"}},
		{Tools: []string{"fly", "flyctl"}, Subs: []string{"deploy", "launch", "scale", "secrets set", "secrets unset", "secrets import", "apps create", "apps restart", "machine run", "machine start", "machine stop", "machine update", "volumes create", "ips allocate-v4", "ips allocate-v6", "certs add"}},
		{Tools: []string{"heroku"}, Subs: []string{"create", "apps:create", "config:set", "config:unset", "ps:scale", "ps:restart", "ps:stop", "releases:rollback", "run", "run:detached", "addons:create", "maintenance:on", "maintenance:off", "domains:add", "pipelines:promote", "container:push", "container:release"}},
		{Tools: []string{"firebase"}, Subs: []string{"deploy", "hosting:channel:deploy", "functions:delete", "functions:config:set", "database:set", "database:remove", "firestore:delete", "use"}},
		{Tools: []string{"wrangler"}, Subs: []string{"deploy", "publish", "secret", "versions deploy", "rollback", "kv key put", "kv bulk put", "r2 object put", "d1 execute", "d1 migrations apply", "pages deploy", "pages publish"}},
		{Tools: []string{"railway"}, Subs: []string{"up", "deploy", "redeploy", "variables", "link"}},
		{Tools: []string{"kamal"}, Subs: []string{"deploy", "setup", "redeploy", "rollback", "remove", "app", "accessory", "env push"}},
		{Tools: []string{"cap", "mina"}, Args: []string{"deploy"}, ArgPrefixes: []string{"deploy:"}},
		{Tools: []string{"gcloud"}, Subs: []string{"app deploy", "functions deploy", "run deploy", "run services update", "builds submit", "deploy releases create"}},
		{Tools: []string{"az"}, Subs: []string{"webapp deploy", "webapp up", "functionapp deploy", "staticwebapp create", "containerapp up", "containerapp update", "spring app deploy"}},
		{Tools: []string{"eb"}, Subs: []string{"deploy", "create", "terminate", "scale", "setenv", "swap"}},
		{Tools: []string{"serverless", "sls"}, Subs: []string{"deploy"}},
		{Tools: []string{"doctl"}, Subs: []string{"apps create", "apps update", "apps create-deployment"}},
		{Tools: []string{"render"}, Subs: []string{"deploys create", "services restart"}},
		{Tools: []string{"surge", "now"}},
	}},
	{ID: "cloud-write", Category: catInfrastructure, Description: "creating or changing cloud resources", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"aws"}, Match: awsWrite},
		{Tools: []string{"gcloud", "az", "doctl", "oci", "ibmcloud", "hcloud", "linode-cli", "vultr-cli", "scw", "exo"}, Match: verbCloudWrite},
	}},
}

var vercelReadCommands = map[string]bool{"dev": true, "env": true, "login": true, "logout": true, "whoami": true, "ls": true, "list": true, "inspect": true, "logs": true, "link": true, "pull": true, "build": true, "help": true, "init": true, "switch": true, "teams": true, "domains": true, "dns": true, "certs": true, "project": true, "projects": true, "git": true, "bisect": true, "secrets": true, "alias": true, "integration": true, "telemetry": true}

// vercelDeploys matches a vercel call that deploys: bare, a path, or deploy, promote, redeploy and rollback.
func vercelDeploys(call shellCall, _ matchEnv) (string, bool) {
	if containsArg(call.Args, "--version", "-v", "--help", "-h") {
		return "", false
	}
	words := positionals(call)
	if len(words) == 0 {
		return "Matched: vercel with no command, which deploys.", true
	}
	if vercelReadCommands[words[0]] || words[0] == "remove" || words[0] == "rm" {
		return "", false
	}
	return "Matched: vercel " + words[0] + ".", true
}

var awsReadPrefixes = []string{"describe", "get", "list", "ls", "help", "wait", "show", "head", "lookup", "search", "scan", "query", "batch-get", "filter", "test-", "simulate-", "validate", "estimate", "preview", "select", "check", "detect", "verify", "download", "export"}

// awsWrite matches an aws action that is not a read. The s3 copy forms are cloud-download, deletions cloud-delete, and credential and login actions their own rules.
func awsWrite(call shellCall, _ matchEnv) (string, bool) {
	words := positionals(call)
	if len(words) < 2 {
		return "", false
	}
	service, action := words[0], words[1]
	switch service {
	case "configure", "sso", "help":
		return "", false
	case "s3":
		if action == "cp" || action == "sync" || action == "mv" || action == "ls" || action == "presign" {
			return "", false
		}
	case "sts":
		return "", false
	}
	for _, prefix := range awsReadPrefixes {
		if strings.HasPrefix(action, prefix) {
			return "", false
		}
	}
	return "Matched: aws " + service + " " + action + ", which is not a read.", true
}

var cloudWriteVerbs = map[string]bool{
	"create": true, "update": true, "deploy": true, "set": true, "add": true, "remove": true, "start": true, "stop": true, "restart": true,
	"reset": true, "resize": true, "suspend": true, "resume": true, "patch": true, "import": true, "enable": true, "disable": true, "apply": true,
	"attach": true, "detach": true, "move": true, "rollback": true, "promote": true, "cancel": true, "submit": true, "deallocate": true,
	"assign": true, "invoke": true, "scale": true, "swap": true, "upload": true, "purge": true, "regenerate": true, "approve": true,
	"reboot": true, "power-on": true, "power-off": true, "shutdown": true, "upgrade": true, "rotate": true, "grant": true, "revoke": true,
	"add-iam-policy-binding": true, "set-iam-policy": true, "unset": true, "run": true, "execute": true, "publish": true, "tag": true, "untag": true,
	"generate-sas": true, "copy": true, "restore": true, "failover": true, "migrate": true, "rebuild": true, "snapshot": true,
}

var cloudWritePrefixes = []string{"set-", "add-", "remove-", "update-", "create-", "start-", "stop-", "enable-", "disable-", "attach-", "detach-", "reset-", "rotate-", "import-"}

// verbCloudWrite matches a cloud CLI call whose command path holds a write verb. The first word is the command group, so a group named like a verb, such as gcloud run, is not read as one.
func verbCloudWrite(call shellCall, _ matchEnv) (string, bool) {
	words := positionals(call)
	if len(words) > 0 && (words[0] == "auth" || words[0] == "login" || words[0] == "account" || words[0] == "config" && len(words) > 1 && words[1] != "set") {
		return "", false
	}
	start := 1
	if len(words) > 1 && (words[0] == "alpha" || words[0] == "beta" || words[0] == "preview") {
		start = 2
	}
	for i := start; i < len(words); i++ {
		word := words[i]
		if cloudWriteVerbs[word] {
			return "Matched: " + call.Name + " " + strings.Join(words[:i+1], " ") + ".", true
		}
		for _, prefix := range cloudWritePrefixes {
			if strings.HasPrefix(word, prefix) {
				return "Matched: " + call.Name + " " + strings.Join(words[:i+1], " ") + ".", true
			}
		}
	}
	return "", false
}
