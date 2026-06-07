package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ParsesExample(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	data := []byte(`{
      "mcpServers": {
        "weather": {
          "command": "uv",
          "args": ["run", "~/src/mcp-servers/weather.py"]
        },
        "tavily": {
          "command": "npx",
          "args": ["-y", "mcp-remote", "https://example.com"],
          "env": {"KEY": "x"}
        }
      }
    }`)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(p, "/home/amir", "amir")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("server count = %d, want 2", len(cfg.Servers))
	}

	var weather, tavily *ServerSpec
	for i, s := range cfg.Servers {
		switch s.Name {
		case "weather":
			weather = &cfg.Servers[i]
		case "tavily":
			tavily = &cfg.Servers[i]
		}
	}
	if weather == nil || tavily == nil {
		t.Fatalf("missing servers: %+v", cfg.Servers)
	}

	if weather.Args[1] != "/home/amir/src/mcp-servers/weather.py" {
		t.Errorf("expand failed: %q", weather.Args[1])
	}
	if tavily.Env["KEY"] != "x" {
		t.Errorf("env not parsed: %v", tavily.Env)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/no/such/file", "/h", "u")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
