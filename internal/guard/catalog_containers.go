// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var containerEngines = []string{"docker", "podman", "nerdctl"}

var composeTools = []string{"docker-compose", "podman-compose"}

var kubeTools = []string{"kubectl", "oc"}

var containerRules = []rule{
	{ID: "docker-destroy", Category: catContainers, Description: "removing docker containers, images, volumes or networks", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"docker", "podman"}, Subs: []string{"rm", "rmi", "kill", "* rm", "* prune"}},
		{Tools: []string{"nerdctl"}, Subs: []string{"rm", "rmi", "kill", "* rm", "* prune"}},
		{Tools: containerEngines, Subs: []string{"system reset", "* remove", "container kill"}},
		{Tools: containerEngines, Subs: []string{"compose down"}, Flags: []string{"-v", "--volumes", "--rmi"}},
		{Tools: composeTools, Subs: []string{"rm", "kill"}},
		{Tools: composeTools, Subs: []string{"down"}, Flags: []string{"-v", "--volumes", "--rmi"}},
		{Tools: []string{"buildah"}, Subs: []string{"rm", "rmi", "prune"}},
		{Tools: []string{"crictl"}, Subs: []string{"rm", "rmi", "rmp", "stop", "stopp"}},
		{Tools: []string{"ctr"}, Subs: []string{"* rm", "* remove", "* delete", "* del", "* kill"}},
	}},
	{ID: "kubectl-delete", Category: catContainers, Description: "deleting kubernetes resources", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"kubectl"}, Subs: []string{"delete", "drain", "cordon"}},
		{Tools: []string{"oc"}, Subs: []string{"delete", "drain", "cordon", "adm drain", "adm cordon", "delete-project"}},
		{Tools: []string{"kind"}, Subs: []string{"delete"}},
		{Tools: []string{"minikube"}, Subs: []string{"delete"}},
		{Tools: []string{"k3d"}, Subs: []string{"cluster delete", "node delete", "registry delete"}},
		{Tools: []string{"helm"}, Subs: []string{"uninstall", "delete", "del", "un", "rollback"}},
		{Tools: []string{"argocd"}, Subs: []string{"app delete", "proj delete", "cluster rm", "repo rm"}},
		{Tools: []string{"skaffold"}, Subs: []string{"delete"}},
	}},

	{ID: "container-control", Category: catContainers, Description: "starting, stopping or changing containers and local clusters", Mode: ModeAsk, Forms: []form{
		{Tools: containerEngines, Subs: []string{"stop", "start", "restart", "pause", "unpause", "update", "rename", "container stop", "container start", "container restart", "container pause", "container unpause", "container update", "container rename"}},
		{Tools: containerEngines, Subs: []string{"compose stop", "compose start", "compose restart", "compose down", "compose pause", "compose unpause", "compose kill", "compose scale"}},
		{Tools: containerEngines, Subs: []string{"swarm init", "swarm join", "swarm leave", "swarm update", "service create", "service update", "service scale", "service rollback", "stack deploy", "network create", "network connect", "network disconnect", "volume create", "context use", "context create", "plugin install", "plugin enable", "plugin disable"}},
		{Tools: composeTools, Subs: []string{"stop", "start", "restart", "down", "pause", "unpause", "scale"}},
		{Tools: []string{"podman"}, Subs: []string{"machine start", "machine stop", "machine init", "machine set", "pod start", "pod stop", "pod restart", "pod create"}},
		{Tools: []string{"kind"}, Subs: []string{"create"}},
		{Tools: []string{"minikube"}, Subs: []string{"start", "stop", "pause", "unpause", "addons enable", "addons disable"}},
		{Tools: []string{"k3d"}, Subs: []string{"cluster create", "cluster start", "cluster stop", "node create", "registry create"}},
		{Tools: []string{"colima", "lima", "limactl"}, Subs: []string{"start", "stop", "restart", "create", "delete"}},
	}},
	{ID: "registry-push", Category: catContainers, Description: "pushing or changing images in a registry", Mode: ModeDeny, Forms: []form{
		{Tools: containerEngines, Subs: []string{"push", "image push", "manifest push"}},
		{Tools: containerEngines, Subs: []string{"build", "buildx build", "image build", "builder build", "buildx bake"}, Flags: []string{"--push"}},
		{Tools: composeTools, Subs: []string{"push"}},
		{Tools: containerEngines, Subs: []string{"compose push"}},
		{Tools: []string{"buildah"}, Subs: []string{"push", "manifest push"}},
		{Tools: []string{"crane"}, Subs: []string{"push", "copy", "cp", "tag", "delete", "mutate", "append", "rebase", "flatten", "index append"}},
		{Tools: []string{"oras"}, Subs: []string{"push", "tag", "attach", "copy", "cp", "manifest push", "manifest delete", "blob push", "blob delete"}},
		{Tools: []string{"helm"}, Subs: []string{"push"}},
		{Tools: []string{"skopeo"}, Subs: []string{"delete"}},
		{Tools: []string{"skopeo"}, Subs: []string{"copy", "sync"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			words := positionals(call)
			if len(words) < 3 {
				return "", false
			}
			dest := words[len(words)-1]
			if strings.HasPrefix(dest, "docker://") {
				return "Matched: skopeo writing to the registry " + dest + ".", true
			}
			return "", false
		}},
	}},
	{ID: "kube-write", Category: catContainers, Description: "changing kubernetes resources or running commands in a cluster", Mode: ModeAsk, Forms: []form{
		{Tools: kubeTools, Subs: []string{"apply", "create", "replace", "patch", "edit", "scale", "rollout", "set", "label", "annotate", "expose", "autoscale", "taint", "uncordon", "run", "cp", "exec", "attach", "debug", "certificate"}},
		{Tools: []string{"kustomize"}, Subs: []string{"edit"}},
		{Tools: []string{"helm"}, Subs: []string{"install", "upgrade", "i"}},
		{Tools: []string{"argocd"}, Subs: []string{"app sync", "app create", "app set", "app patch", "app rollback", "app terminate-op", "app actions run", "proj create", "cluster add", "repo add"}},
		{Tools: []string{"flux"}, Subs: []string{"reconcile", "suspend", "resume", "create", "delete", "bootstrap", "install", "uninstall"}},
		{Tools: []string{"skaffold"}, Subs: []string{"run", "deploy", "dev", "apply"}},
		{Tools: []string{"tilt"}, Subs: []string{"up", "down", "ci"}},
	}},
}
