package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed embedded/skill.md
var embeddedSkill embed.FS

//go:embed embedded/session-start.sh
var embeddedSessionStart []byte

//go:embed embedded/hooks.json
var embeddedHooksJSON []byte

const (
	pluginName = "claude-term"
	pluginKey  = "claude-term@local-tools"
	// hookMarker identifies old standalone claude-term entries in settings.json.
	hookMarker = "claude-term"
)

func claudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func cmdInstall() error {
	claudeBase := claudeDir()
	home, _ := os.UserHomeDir()
	cacheBase := filepath.Join(claudeBase, "plugins", "cache", "local-tools", pluginName)
	installedPath := filepath.Join(claudeBase, "plugins", "installed_plugins.json")

	// Read current version from source plugin.json, bump it
	srcPluginJSON := findSourcePluginJSON()
	currentVersion := readSourceVersion(srcPluginJSON)
	version := "0.1.0"
	if currentVersion != "" {
		version = bumpPatch(currentVersion)
	}
	// Write bumped version back to source plugin.json
	if srcPluginJSON != "" {
		writeSourceVersion(srcPluginJSON, version)
		fmt.Printf("  Version: %s → %s\n", currentVersion, version)
	}

	cacheDir := filepath.Join(cacheBase, version)

	// Remove old cached versions
	if entries, err := os.ReadDir(cacheBase); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				os.RemoveAll(filepath.Join(cacheBase, e.Name()))
			}
		}
	}

	// Write plugin files to cache
	skillContent, err := embeddedSkill.ReadFile("embedded/skill.md")
	if err != nil {
		return fmt.Errorf("read embedded skill: %w", err)
	}

	files := map[string]struct {
		content []byte
		perm    os.FileMode
	}{
		".claude-plugin/plugin.json":  {pluginJSON(version), 0o644},
		"skills/claude-term/SKILL.md": {skillContent, 0o644},
		"hooks/hooks.json":            {embeddedHooksJSON, 0o644},
		"hooks/session-start.sh":      {embeddedSessionStart, 0o755},
	}

	for relPath, f := range files {
		dest := filepath.Join(cacheDir, relPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(dest, f.content, f.perm); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}
	fmt.Printf("  ✅ Plugin files cached (v%s)\n", version)

	// Register in installed_plugins.json
	if err := registerPlugin(installedPath, cacheDir, version); err != nil {
		return fmt.Errorf("register plugin: %w", err)
	}
	fmt.Println("  ✅ Registered in installed_plugins.json")

	// Ensure owners directory exists
	termDir := filepath.Join(home, ".claude-term")
	if err := os.MkdirAll(filepath.Join(termDir, "owners"), 0o700); err != nil {
		return fmt.Errorf("create owners dir: %w", err)
	}

	// Clean up old standalone hooks from settings.json (if any)
	settingsPath := filepath.Join(claudeBase, "settings.json")
	if removedStandalone := cleanupStandaloneHooks(settingsPath); removedStandalone {
		fmt.Println("  ✅ Removed old standalone hooks from settings.json")
		// Remove old standalone skill
		oldSkillDir := filepath.Join(claudeBase, "skills", "claude-term")
		os.RemoveAll(oldSkillDir)
	}

	fmt.Println("\n✅ claude-term plugin installed!")
	fmt.Println("   Run /reload-plugins in active sessions, or start a new session.")
	return nil
}

func cmdUninstall() error {
	claudeBase := claudeDir()
	home, _ := os.UserHomeDir()
	cacheBase := filepath.Join(claudeBase, "plugins", "cache", "local-tools", pluginName)
	installedPath := filepath.Join(claudeBase, "plugins", "installed_plugins.json")

	// Remove from installed_plugins.json
	if err := unregisterPlugin(installedPath); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Could not unregister plugin: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed from installed_plugins.json")
	}

	// Remove cached files
	if err := os.RemoveAll(cacheBase); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Could not remove cache: %v\n", err)
	} else {
		fmt.Println("  ✅ Plugin cache removed")
	}

	// Also clean up any old standalone hooks
	settingsPath := filepath.Join(claudeBase, "settings.json")
	if removedStandalone := cleanupStandaloneHooks(settingsPath); removedStandalone {
		fmt.Println("  ✅ Removed old standalone hooks from settings.json")
	}

	// Remove old standalone files
	oldSkillDir := filepath.Join(claudeBase, "skills", "claude-term")
	os.RemoveAll(oldSkillDir)
	oldHookDir := filepath.Join(home, ".claude-term", "hooks")
	os.RemoveAll(oldHookDir)

	fmt.Println("\n✅ claude-term uninstalled.")
	fmt.Println("   Terminal state preserved in ~/.claude-term/ (delete manually if unwanted).")
	return nil
}

// --- Plugin JSON generation ---

func pluginJSON(version string) []byte {
	m := map[string]interface{}{
		"name":        pluginName,
		"description": "Persistent terminal management — spawn, read, write, and kill terminals that survive session disconnects",
		"version":     version,
		"author":      map[string]string{"name": "Elias Schlie"},
		"repository":  "https://github.com/EliasSchlie/claude-term",
		"license":     "MIT",
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	return append(out, '\n')
}

// --- installed_plugins.json management ---

func registerPlugin(path, cacheDir, version string) error {
	data := map[string]interface{}{"version": 2, "plugins": map[string]interface{}{}}
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &data)
	}

	plugins, _ := data["plugins"].(map[string]interface{})
	if plugins == nil {
		plugins = map[string]interface{}{}
		data["plugins"] = plugins
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	entry := map[string]interface{}{
		"scope":       "user",
		"installPath": cacheDir,
		"version":     version,
		"lastUpdated": now,
	}

	// Preserve installedAt from existing entry
	if existing, ok := plugins[pluginKey].([]interface{}); ok && len(existing) > 0 {
		if e, ok := existing[0].(map[string]interface{}); ok {
			if at, ok := e["installedAt"].(string); ok {
				entry["installedAt"] = at
			}
		}
	}
	if _, ok := entry["installedAt"]; !ok {
		entry["installedAt"] = now
	}

	plugins[pluginKey] = []interface{}{entry}
	return writeJSON(path, data)
}

func unregisterPlugin(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	plugins, _ := data["plugins"].(map[string]interface{})
	delete(plugins, pluginKey)
	return writeJSON(path, data)
}

// --- Standalone hook cleanup ---

func cleanupStandaloneHooks(settingsPath string) bool {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	hooks, _ := data["hooks"].(map[string]interface{})
	if hooks == nil {
		return false
	}

	entries, ok := hooks["SessionStart"].([]interface{})
	if !ok {
		return false
	}

	removed := false
	var filtered []interface{}
	for _, entry := range entries {
		entryMap, _ := entry.(map[string]interface{})
		hooksList, _ := entryMap["hooks"].([]interface{})
		isCT := false
		for _, h := range hooksList {
			hMap, _ := h.(map[string]interface{})
			if cmd, _ := hMap["command"].(string); strings.Contains(cmd, hookMarker) && !strings.Contains(cmd, "CLAUDE_PLUGIN_ROOT") {
				isCT = true
				break
			}
		}
		if isCT {
			removed = true
		} else {
			filtered = append(filtered, entry)
		}
	}

	if removed {
		if len(filtered) == 0 {
			delete(hooks, "SessionStart")
		} else {
			hooks["SessionStart"] = filtered
		}
		writeJSON(settingsPath, data)
	}
	return removed
}

// --- Source plugin.json management ---

// findSourcePluginJSON looks for .claude-plugin/plugin.json relative to the
// running binary's directory, walking up to find the repo root.
func findSourcePluginJSON() string {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for range 5 {
			candidate := filepath.Join(dir, ".claude-plugin", "plugin.json")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			dir = filepath.Dir(dir)
		}
	}
	// Try relative to working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, ".claude-plugin", "plugin.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func readSourceVersion(path string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, _ := m["version"].(string)
	return v
}

func writeSourceVersion(path, version string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	m["version"] = version
	out, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(path, append(out, '\n'), 0o644)
}

func bumpPatch(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return version
	}
	patch := 0
	fmt.Sscanf(parts[2], "%d", &patch)
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}

func writeJSON(path string, data map[string]interface{}) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
