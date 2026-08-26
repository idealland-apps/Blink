package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{Provider: "miniflux", URL: "https://rss.example.test", Username: "anthony", Token: "secret"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestValidateRequiresSupportedProviderAndCredentials(t *testing.T) {
	if err := (Config{Provider: "other", URL: "https://rss.example.test", Token: "x"}).Validate(); err == nil {
		t.Fatal("Validate accepted unsupported provider")
	}
	if err := (Config{Provider: "miniflux", URL: "https://rss.example.test"}).Validate(); err == nil {
		t.Fatal("Validate accepted empty credentials")
	}
}
