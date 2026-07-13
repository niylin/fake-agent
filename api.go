package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type App struct {
	manager *Manager
}

func NewApp(manager *Manager) *App {
	return &App{manager: manager}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/api/default", a.handleDefault)
	mux.HandleFunc("/api/templates", a.handleTemplates)
	mux.HandleFunc("/api/agents", a.handleAgents)
	mux.HandleFunc("/api/agents/", a.handleAgent)
	return mux
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": appVersion,
	})
}

func (a *App) handleDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, defaultAgentConfig())
}

func (a *App) handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, templates())
}

func (a *App) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.manager.List())
	case http.MethodPost:
		var cfg AgentConfig
		if err := readJSON(w, r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		view, err := a.manager.Create(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, view)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleAgent(w http.ResponseWriter, r *http.Request) {
	parts := splitAgentPath(r.URL.Path)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "agent id is required")
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		a.handleAgentRecord(w, r, id)
		return
	}
	if len(parts) == 2 {
		a.handleAgentAction(w, r, id, parts[1])
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (a *App) handleAgentRecord(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPut:
		var cfg AgentConfig
		if err := readJSON(w, r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		view, err := a.manager.Update(id, cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		if err := a.manager.Delete(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleAgentAction(w http.ResponseWriter, r *http.Request, id, action string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch action {
	case "start":
		if err := a.manager.Start(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
	case "stop":
		if err := a.manager.Stop(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	default:
		writeError(w, http.StatusNotFound, "unknown action")
	}
}

func splitAgentPath(path string) []string {
	rest := strings.TrimPrefix(path, "/api/agents/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": fmt.Sprintf("%s", message),
	})
}
