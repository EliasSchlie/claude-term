package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EliasSchlie/claude-term/internal/paths"
)

//go:embed embedded/skill.md
//go:embed embedded/session-start.sh
var embeddedFiles embed.FS

const hookMarker = "claude-term"

func claudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func cmdInstall() error {
	claudeBase := claudeDir()
	termDir := paths.Dir()

	// 1. Install skill
	skillDir := filepath.Join(claudeBase, "skills", "claude-term")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	skillContent, err := embeddedFiles.ReadFile("embedded/skill.md")
	if err != nil {
		return fmt.Errorf("read embedded skill: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContent, 0o644); err != nil {
		return fmt.Errorf("write skill: %w", err)
	}
	fmt.Println("  ✅ Skill installed")

	// 2. Install hook script
	hookDir := filepath.Join(termDir, "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return fmt.Errorf("create hook dir: %w", err)
	}
	hookScript, err := embeddedFiles.ReadFile("embedded/session-start.sh")
	if err != nil {
		return fmt.Errorf("read embedded hook: %w", err)
	}
	hookPath := filepath.Join(hookDir, "session-start.sh")
	if err := os.WriteFile(hookPath, hookScript, 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}

	// 3. Register hook in settings.json
	settingsPath := filepath.Join(claudeBase, "settings.json")
	hookCmd := fmt.Sprintf("bash %s", hookPath)
	if err := addHookToSettings(settingsPath, hookCmd); err != nil {
		return fmt.Errorf("register hook: %w", err)
	}
	fmt.Println("  ✅ SessionStart hook installed")

	// 4. Ensure owners directory exists
	if err := os.MkdirAll(filepath.Join(termDir, "owners"), 0o700); err != nil {
		return fmt.Errorf("create owners dir: %w", err)
	}

	fmt.Println("\n✅ claude-term installed (standalone mode)!")
	fmt.Println("   Start a new Claude session to activate.")
	fmt.Println("   Note: If using the claude-term plugin, this step is unnecessary.")
	return nil
}

func cmdUninstall() error {
	claudeBase := claudeDir()

	// 1. Remove skill
	skillDir := filepath.Join(claudeBase, "skills", "claude-term")
	if err := os.RemoveAll(skillDir); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Could not remove skill: %v\n", err)
	} else {
		fmt.Println("  ✅ Skill removed")
	}

	// 2. Remove hook from settings.json
	settingsPath := filepath.Join(claudeBase, "settings.json")
	if err := removeHookFromSettings(settingsPath); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Could not remove hook: %v\n", err)
	} else {
		fmt.Println("  ✅ Hook removed from settings.json")
	}

	// 3. Remove hook script (but keep owners/state for potential re-install)
	hookPath := filepath.Join(paths.Dir(), "hooks")
	_ = os.RemoveAll(hookPath)

	fmt.Println("\n✅ claude-term uninstalled.")
	fmt.Println("   Terminal state preserved in ~/.claude-term/ (delete manually if unwanted).")
	return nil
}

// addHookToSettings adds the claude-term SessionStart hook to settings.json.
// Creates settings.json if it doesn't exist. Does nothing if already installed.
func addHookToSettings(path string, hookCmd string) error {
	data := map[string]interface{}{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
	}

	hooks, _ := data["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
		data["hooks"] = hooks
	}

	// Check if already installed
	if entries, ok := hooks["SessionStart"].([]interface{}); ok {
		for _, entry := range entries {
			entryMap, _ := entry.(map[string]interface{})
			hooksList, _ := entryMap["hooks"].([]interface{})
			for _, h := range hooksList {
				hMap, _ := h.(map[string]interface{})
				if cmd, _ := hMap["command"].(string); cmd != "" {
					if strings.Contains(cmd, hookMarker) {
						return nil // already installed
					}
				}
			}
		}
	}

	// Add hook
	entries, _ := hooks["SessionStart"].([]interface{})
	entries = append(entries, map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": hookCmd,
			},
		},
	})
	hooks["SessionStart"] = entries

	return writeJSON(path, data)
}

// removeHookFromSettings removes claude-term hooks from settings.json.
func removeHookFromSettings(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil // no settings.json, nothing to remove
	}

	data := map[string]interface{}{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	hooks, _ := data["hooks"].(map[string]interface{})
	if hooks == nil {
		return nil
	}

	entries, ok := hooks["SessionStart"].([]interface{})
	if !ok {
		return nil
	}

	// Filter out claude-term entries
	var filtered []interface{}
	for _, entry := range entries {
		entryMap, _ := entry.(map[string]interface{})
		hooksList, _ := entryMap["hooks"].([]interface{})
		isCT := false
		for _, h := range hooksList {
			hMap, _ := h.(map[string]interface{})
			if cmd, _ := hMap["command"].(string); strings.Contains(cmd, hookMarker) {
				isCT = true
				break
			}
		}
		if !isCT {
			filtered = append(filtered, entry)
		}
	}

	if len(filtered) == 0 {
		delete(hooks, "SessionStart")
	} else {
		hooks["SessionStart"] = filtered
	}

	return writeJSON(path, data)
}

func writeJSON(path string, data map[string]interface{}) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
