package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCreateAppendsAgents(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(NewManager(store)).Routes()

	for _, name := range []string{"one", "two"} {
		cfg := defaultAgentConfig()
		cfg.Name = name
		cfg.Endpoint = "https://komari.example.com"
		cfg.DiscoveryKey = "123456789012"
		cfg.Enabled = false
		body, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s status = %d body = %s", name, rec.Code, rec.Body.String())
		}
	}

	agents := store.All()
	if len(agents) != 2 {
		t.Fatalf("agent count = %d, want 2", len(agents))
	}
	if agents[0].Name != "one" || agents[1].Name != "two" {
		t.Fatalf("agents = %+v", agents)
	}
}
