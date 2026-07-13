package main

import (
	"path/filepath"
	"testing"
)

func TestResetPanelPassword(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	password, err := store.ResetPanelPassword("0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if password != "0123456789ab" {
		t.Fatalf("password = %q", password)
	}
	if !verifyPassword(password, store.PanelPasswordHash()) {
		t.Fatal("reset password does not verify")
	}
}

func TestResetPanelPasswordGeneratesRandomPassword(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	password, err := store.ResetPanelPassword("")
	if err != nil {
		t.Fatal(err)
	}
	if len(password) < 12 {
		t.Fatalf("generated password too short: %q", password)
	}
	if !verifyPassword(password, store.PanelPasswordHash()) {
		t.Fatal("generated password does not verify")
	}
}
