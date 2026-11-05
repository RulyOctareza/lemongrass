// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"os"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
)

const claudeProjectDirEnv = "CLAUDE_PROJECT_DIR"

func dangerDecision(payload hookEvent) *guard.Verdict {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	launch := launchProject()
	input := guard.Input{Tool: payload.ToolName, Cwd: payload.Cwd, Home: home, ProjectRoot: launch.Path}
	if command, verdict, isShell := shellToolCommand(payload); isShell {
		if verdict != nil {
			return verdict
		}
		input.Tool = "Bash"
		input.Command = command
	} else {
		input.Paths = dangerToolPaths(payload)
		input.Writes = isMemoryWriteTool(payload.ToolName)
	}
	return guard.Decide(input, hookPolicy(launch.ID))
}

func dangerToolPaths(payload hookEvent) []string {
	paths := memoryToolPaths(payload)
	var generic map[string]any
	if err := json.Unmarshal(payload.ToolInput, &generic); err != nil {
		return paths
	}
	keys := []string{"path", "glob"}
	if payload.ToolName == "Glob" {
		keys = append(keys, "pattern")
	}
	for _, key := range keys {
		if text, ok := generic[key].(string); ok && text != "" {
			paths = append(paths, text)
		}
	}
	return paths
}

// launchProject is the registered project Claude Code was started in, which a cd inside the session does not move. It is the zero Project when there is none.
func launchProject() project.Project {
	dir := os.Getenv(claudeProjectDirEnv)
	if dir == "" {
		return project.Project{}
	}
	proj, err := project.Resolve(dir)
	if err != nil {
		return project.Project{}
	}
	return proj
}
