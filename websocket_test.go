package main

import (
	"encoding/json"
	"testing"
)

func TestBuildRPCURL(t *testing.T) {
	u, err := buildRPCURL("https://komari.example.com/base/", "secret token")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := u.String(), "wss://komari.example.com/base/api/clients/v2/rpc?token=secret+token"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestBuildRPCURLKeepsRPCPath(t *testing.T) {
	u, err := buildRPCURL("ws://127.0.0.1:25774/api/clients/v2/rpc?x=1", "token")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := u.String(), "ws://127.0.0.1:25774/api/clients/v2/rpc?token=token&x=1"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestV2ReportEnvelope(t *testing.T) {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "agent.report",
		Params: map[string]any{
			"report": reportPayload{},
		},
		ID: nil,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["jsonrpc"] != "2.0" || got["method"] != "agent.report" {
		t.Fatalf("unexpected envelope: %s", b)
	}
	params, ok := got["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing: %s", b)
	}
	if _, ok := params["report"]; !ok {
		t.Fatalf("report param missing: %s", b)
	}
	if _, ok := got["id"]; !ok || got["id"] != nil {
		t.Fatalf("id must be present as null: %s", b)
	}
}

func TestV2BasicInfoEnvelope(t *testing.T) {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "agent.basicInfo",
		Params: map[string]any{
			"info": BasicInfo{OS: "Debian"},
		},
		ID: nil,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	params := got["params"].(map[string]any)
	if _, ok := params["info"]; !ok {
		t.Fatalf("info param missing: %s", b)
	}
}
