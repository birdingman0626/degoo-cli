package auth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"degoo-cli/internal/httpclient"
	"github.com/joho/godotenv"
)

const (
	loginURL   = "https://rest-api.degoo.com/login"
	refreshURL = "https://rest-api.degoo.com/access-token/v2"
)

type Credentials struct {
	Email    string
	Password string
}

type TokenCache struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func Login(creds Credentials) (*TokenCache, error) {
	body := map[string]interface{}{
		"Username":      creds.Email,
		"Password":      creds.Password,
		"GenerateToken": true,
	}
	var loginResp struct {
		RefreshToken string `json:"RefreshToken"`
	}
	if err := httpclient.PostJSON(loginURL, nil, body, &loginResp); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if loginResp.RefreshToken == "" {
		return nil, errors.New("login: empty RefreshToken in response")
	}

	// Exchange the refresh token for a short-lived JWT access token.
	accessToken, err := RefreshAccessToken(loginResp.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	return &TokenCache{
		AccessToken:  accessToken,
		RefreshToken: loginResp.RefreshToken,
	}, nil
}

func RefreshAccessToken(refreshToken string) (string, error) {
	var resp struct {
		AccessToken string `json:"AccessToken"`
	}
	// The endpoint expects {"RefreshToken": "<opaque-token>"} and returns a JWT.
	if err := httpclient.PostJSON(refreshURL, nil, map[string]string{"RefreshToken": refreshToken}, &resp); err != nil {
		return "", fmt.Errorf("token refresh: %w", err)
	}
	if resp.AccessToken == "" {
		return "", errors.New("token refresh: empty AccessToken in response")
	}
	return resp.AccessToken, nil
}

func cacheDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "degoo-cli")
	return p, os.MkdirAll(p, 0700)
}

func cacheFile() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keys.json"), nil
}

func SaveTokenCache(cache *TokenCache) error {
	path, err := cacheFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadTokenCache() (*TokenCache, error) {
	path, err := cacheFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache TokenCache
	return &cache, json.Unmarshal(data, &cache)
}

// ResolveCredentials reads from envPath (.env file), falls back to interactive prompt.
func ResolveCredentials(envPath string) (Credentials, error) {
	env, err := godotenv.Read(envPath)
	if err == nil {
		c := Credentials{Email: env["USER"], Password: env["PASSWORD"]}
		if c.Email != "" && c.Password != "" {
			return c, nil
		}
	}
	return promptCredentials()
}

func promptCredentials() (Credentials, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Degoo email: ")
	email, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return Credentials{}, fmt.Errorf("reading email: %w", err)
	}
	fmt.Print("Password: ")
	pass, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return Credentials{}, fmt.Errorf("reading password: %w", err)
	}
	c := Credentials{
		Email:    strings.TrimSpace(email),
		Password: strings.TrimSpace(pass),
	}
	if c.Email == "" || c.Password == "" {
		return Credentials{}, errors.New("credentials required: email and password must not be empty")
	}
	return c, nil
}
