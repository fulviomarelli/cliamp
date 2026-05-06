package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppleMusicDisabledByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppleMusic.Enabled {
		t.Error("AppleMusic.Enabled = true with no config, want false")
	}
	if cfg.AppleMusic.IsSet() {
		t.Error("AppleMusic.IsSet() = true with no config, want false")
	}
	if cfg.AppleMusic.Storefront != "us" {
		t.Errorf("AppleMusic.Storefront = %q, want us", cfg.AppleMusic.Storefront)
	}
}

func TestLoadAppleMusicExplicitlyEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(os.Getenv("HOME"), ".config", "cliamp", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	data := []byte(`
[apple_music]
enabled = true
web_bearer_token = "bearer-token"
media_user_token = "user-token"
storefront = "it"
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AppleMusic.Enabled {
		t.Error("AppleMusic.Enabled = false, want true")
	}
	if cfg.AppleMusic.WebBearerToken != "bearer-token" {
		t.Errorf("AppleMusic.WebBearerToken = %q, want bearer-token", cfg.AppleMusic.WebBearerToken)
	}
	if cfg.AppleMusic.MediaUserToken != "user-token" {
		t.Errorf("AppleMusic.MediaUserToken = %q, want user-token", cfg.AppleMusic.MediaUserToken)
	}
	if cfg.AppleMusic.Storefront != "it" {
		t.Errorf("AppleMusic.Storefront = %q, want it", cfg.AppleMusic.Storefront)
	}
	if !cfg.AppleMusic.IsSet() {
		t.Error("AppleMusic.IsSet() = false, want true")
	}
}

func TestLoadAppleMusicSectionWithoutEnabledStaysOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path := filepath.Join(os.Getenv("HOME"), ".config", "cliamp", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	data := []byte(`
[apple_music]
web_bearer_token = "bearer-token"
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppleMusic.Enabled {
		t.Error("AppleMusic.Enabled = true without explicit enabled = true, want false")
	}
	if cfg.AppleMusic.IsSet() {
		t.Error("AppleMusic.IsSet() = true without explicit enabled = true, want false")
	}
}

func TestLoadAppleMusicInterpolatesTokenFromEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLIAMP_TEST_APPLE_TOKEN", "env-token")

	path := filepath.Join(os.Getenv("HOME"), ".config", "cliamp", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	data := []byte(`
[apple_music]
enabled = true
web_bearer_token = "${CLIAMP_TEST_APPLE_TOKEN}"
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppleMusic.WebBearerToken != "env-token" {
		t.Errorf("AppleMusic.WebBearerToken = %q, want env-token", cfg.AppleMusic.WebBearerToken)
	}
	if cfg.AppleMusic.Storefront != "us" {
		t.Errorf("AppleMusic.Storefront = %q, want us", cfg.AppleMusic.Storefront)
	}
}
