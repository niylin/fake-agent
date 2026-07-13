package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildRegisterURL(t *testing.T) {
	u, err := buildRegisterURL("https://komari.example.com/base/", "node 1")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := u.String(), "https://komari.example.com/base/api/clients/register?name=node+1"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestRegisterByDiscoveryKey(t *testing.T) {
	oldClient := newDiscoveryHTTPClient
	t.Cleanup(func() { newDiscoveryHTTPClient = oldClient })
	newDiscoveryHTTPClient = func(bool) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if r.URL.Path != "/api/clients/register" {
				t.Fatalf("path = %s", r.URL.Path)
			}
			if got, want := r.URL.Query().Get("name"), "node-a"; got != want {
				t.Fatalf("name = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer discovery-secret"; got != want {
				t.Fatalf("authorization = %q, want %q", got, want)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"success","data":{"uuid":"uuid-a","token":"token-a"}}`)),
				Header:     make(http.Header),
			}, nil
		})}
	}

	result, err := RegisterByDiscoveryKey(context.Background(), AgentConfig{
		Name:         "node-a",
		Endpoint:     "https://komari.example.com",
		DiscoveryKey: "discovery-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UUID != "uuid-a" || result.Token != "token-a" {
		t.Fatalf("result = %+v", result)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
