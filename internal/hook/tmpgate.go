package hook

import (
	"os"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/session"
)

// tempWriteDeny denies a file tool writing into the system temp dir in a bibliothek project, since a reboot or power cut wipes it and a file the model writes itself is costly to lose. Shell writes stay open so what a command regenerates lives there instead of wearing the drive.
func tempWriteDeny(payload hookEvent, projectPath string) string {
	if !isMemoryWriteTool(payload.ToolName) {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	in := guard.Input{Tool: payload.ToolName, Cwd: payload.Cwd, Home: home, ProjectRoot: projectPath, Paths: memoryToolPaths(payload), Writes: true}
	target := guard.TempWrite(in, guard.TempRoots())
	if target == "" {
		return ""
	}
	return session.FormatTempWriteDeny(target)
}
