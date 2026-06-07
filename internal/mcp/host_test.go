package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func buildFakeserver(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakeserver")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakeserver")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fakeserver: %v\n%s", err, out)
	}
	return bin
}

func TestHost_ConnectsAndListsTools(t *testing.T) {
	bin := buildFakeserver(t)

	h, err := OpenHost(context.Background(), &Config{
		Servers: []ServerSpec{{Name: "fake", Command: bin}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tools := h.Tools()
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	if tools[0].Name != "fake__echo" {
		t.Errorf("prefixed name = %q", tools[0].Name)
	}
}

func TestHost_CallRoutesToServer(t *testing.T) {
	bin := buildFakeserver(t)
	h, err := OpenHost(context.Background(), &Config{
		Servers: []ServerSpec{{Name: "fake", Command: bin}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	out, err := h.Call(context.Background(), "fake__echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "echo: hi" {
		t.Errorf("got %q, want %q", out, "echo: hi")
	}
}

func TestHost_OneServerFailingDoesNotKillOthers(t *testing.T) {
	bin := buildFakeserver(t)
	h, err := OpenHost(context.Background(), &Config{
		Servers: []ServerSpec{
			{Name: "fake", Command: bin},
			{Name: "broken", Command: "/no/such/binary"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tools := h.Tools()
	if len(tools) != 1 || tools[0].Name != "fake__echo" {
		t.Errorf("got tools %+v, want only fake__echo", tools)
	}
}

func TestHost_StartupTimeoutEnforced(t *testing.T) {
	cfg := &Config{Servers: []ServerSpec{
		{Name: "hang", Command: "sleep", Args: []string{"60"}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := openHostWithBudget(ctx, cfg, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	if len(h.Tools()) != 0 {
		t.Errorf("expected no tools from hung server, got %+v", h.Tools())
	}
}
