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
	cookies := loginForTest(t, app, store.GeneratedPanelPassword())

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
		addCookies(req, cookies)
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

func TestPanelLoginRequired(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(NewManager(store)).Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	cookies := loginForTest(t, app, store.GeneratedPanelPassword())
	req = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	addCookies(req, cookies)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func loginForTest(t *testing.T, app http.Handler, password string) []*http.Cookie {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

func addCookies(req *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
}
