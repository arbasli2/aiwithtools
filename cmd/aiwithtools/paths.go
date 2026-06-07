package main

import "path/filepath"

func configDir(home, xdgConfigHome string) string {
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "aiwithtools")
	}
	return filepath.Join(home, ".config", "aiwithtools")
}

func dataDir(home, xdgDataHome string) string {
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "aiwithtools")
	}
	return filepath.Join(home, ".local", "share", "aiwithtools")
}
