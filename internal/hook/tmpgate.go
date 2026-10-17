package hook

import (
	"os"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/session"
)

// tempWriteDeny denies a write aimed at the system temp dir in a bibliothek project, since a reboot or power cut wipes it, so work files land in biblio/scratchpad instead. Reads stay open because Claude Code itself saves long tool output there.
func tempWriteDeny(payload hookEvent, projectPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	in := guard.Input{Tool: payload.ToolName, Cwd: payload.Cwd, Home: home, ProjectRoot: projectPath}
	if command := shellCommand(payload.ToolName, payload.ToolInput); command != "" {
		in.Command = command
	} else if isMemoryWriteTool(payload.ToolName) {
		in.Paths = memoryToolPaths(payload)
		in.Writes = true
	} else {
		return ""
	}
	target := guard.TempWrite(in, guard.TempRoots())
	if target == "" {
		return ""
	}
	return session.FormatTempWriteDeny(target)
}
