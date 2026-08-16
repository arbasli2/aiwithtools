package main

import "path/filepath"

func configDir(home, xdgConfigHome string) string {
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "aiwithtools")
	}
	return filepath.Join(home, ".config", "aiwithtools")
}

// mcpConfigPath resolves the MCP config location. An explicit --mcp
// path wins verbatim — relative paths resolve against the working
// directory, not cfgDir — otherwise it defaults to mcp.json inside the
// config dir.
func mcpConfigPath(cfgDir, mcpFile string) string {
	if mcpFile != "" {
		return mcpFile
	}
	return filepath.Join(cfgDir, "mcp.json")
}

func dataDir(home, xdgDataHome string) string {
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "aiwithtools")
	}
	return filepath.Join(home, ".local", "share", "aiwithtools")
}
