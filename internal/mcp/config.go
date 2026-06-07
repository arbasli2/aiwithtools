package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type Config struct {
	Servers []ServerSpec
}

type ServerSpec struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

type rawConfig struct {
	MCPServers map[string]rawServer `json:"mcpServers"`
}

type rawServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func LoadConfig(path, home, user string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mcp config: %w", err)
	}
	var rc rawConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}

	cfg := &Config{}
	for name, rs := range rc.MCPServers {
		spec := ServerSpec{
			Name:    name,
			Command: expandPath(rs.Command, home, user),
			Env:     rs.Env,
		}
		for _, a := range rs.Args {
			spec.Args = append(spec.Args, expandPath(a, home, user))
		}
		cfg.Servers = append(cfg.Servers, spec)
	}
	sort.Slice(cfg.Servers, func(i, j int) bool { return cfg.Servers[i].Name < cfg.Servers[j].Name })
	return cfg, nil
}
