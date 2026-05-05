package auth_test

import (
	"os"
	"testing"

	"degoo-cli/internal/auth"
)

func TestTokenCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir) // Windows config dir

	cache := &auth.TokenCache{
		AccessToken:  "acc123",
		RefreshToken: "ref456",
	}
	if err := auth.SaveTokenCache(cache); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := auth.LoadTokenCache()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AccessToken != cache.AccessToken || loaded.RefreshToken != cache.RefreshToken {
		t.Errorf("got %+v, want %+v", loaded, cache)
	}
}

func TestLoginRealAPI(t *testing.T) {
	email := os.Getenv("DEGOO_USER")
	pass := os.Getenv("DEGOO_PASS")
	if email == "" || pass == "" {
		t.Skip("DEGOO_USER/DEGOO_PASS not set")
	}
	tokens, err := auth.Login(auth.Credentials{Email: email, Password: pass})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}
	// Save token for subsequent API tests
	if err := auth.SaveTokenCache(tokens); err != nil {
		t.Logf("warning: could not save token cache: %v", err)
	}
}
