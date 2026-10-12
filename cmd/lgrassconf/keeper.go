package main

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/faizalv/lemongrass/internal/agentconf"
	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

//go:embed skills
var skillsFS embed.FS

var legacySkillNames = []string{"lemongrass", "lgrass-staleness", "lgrass-closing", "lgrass-copilot", "lgrass-pilot"}

type vendor struct {
	embedRoot string
	configDir string
}

var (
	claude  = vendor{embedRoot: "skills/claude", configDir: ".claude"}
	codex   = vendor{embedRoot: "skills/codex", configDir: ".codex"}
	vendors = []vendor{claude, codex}
)

type skillFile struct {
	root    string
	path    string
	content []byte
}

type keeper struct {
	home        string
	candidates  []string
	skills      []skillFile
	orientation []skillFile
	plugin      []skillFile
}

func newKeeper(home string) *keeper {
	k := &keeper{
		home: home,
		candidates: []string{
			filepath.Join(home, ".lemongrass", "bin", "lgrassd"),
		},
	}
	k.skills = k.loadSkills()
	k.orientation = k.loadOrientation()
	k.plugin = k.loadPlugin()
	return k
}

func (k *keeper) loadSkills() []skillFile {
	var files []skillFile
	for _, v := range vendors {
		root := k.skillsRoot(v)
		err := fs.WalkDir(skillsFS, v.embedRoot, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := skillsFS.ReadFile(p)
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(p, v.embedRoot+"/")
			files = append(files, skillFile{root: root, path: filepath.Join(root, filepath.FromSlash(rel)), content: data})
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	return files
}

func (k *keeper) skillsRoot(v vendor) string {
	return filepath.Join(k.home, v.configDir, "skills")
}

func (k *keeper) legacySkillDir(v vendor, name string) string {
	return filepath.Join(k.skillsRoot(v), name)
}

func (k *keeper) accounts() claudeaccount.Accounts {
	return claudeaccount.Accounts{Home: k.home, HookBinary: k.lgrassdPath()}
}

func (k *keeper) claudeSettingsPath() string {
	return filepath.Join(k.home, ".claude", "settings.json")
}

func (k *keeper) codexHooksPath() string {
	return filepath.Join(k.home, ".codex", "hooks.json")
}

func (k *keeper) lgrassdPath() string {
	for _, c := range k.candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func removeLegacySkill(dir string) error {
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
}

func (k *keeper) reconcile() error {
	var errs []error
	for _, f := range k.managedFiles() {
		if err := agentconf.SyncFile(f.path, f.content); err != nil {
			errs = append(errs, err)
		}
	}
	for _, v := range vendors {
		for _, name := range legacySkillNames {
			if err := removeLegacySkill(k.legacySkillDir(v, name)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if path := k.lgrassdPath(); path != "" {
		if err := agentconf.SyncHookConfig(k.claudeSettingsPath(), path, agentconf.ReconcileClaudeSettings); err != nil {
			errs = append(errs, err)
		}
		if err := agentconf.SyncHookConfig(k.codexHooksPath(), path, reconcileCodexHooks); err != nil {
			errs = append(errs, err)
		}
	}
	if err := k.accounts().Reconcile(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
