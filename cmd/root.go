package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"degoo-cli/internal/api"
	"degoo-cli/internal/auth"
	"degoo-cli/internal/logger"
)

var (
	flagEnvPath string
	flagLogPath string
	apiClient   *api.Client
	log         *logger.Logger
)

var rootCmd = &cobra.Command{
	Use:   "degoo-cli",
	Short: "Upload and download files to Degoo cloud storage",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		exe, _ := os.Executable()
		exeDir := filepath.Dir(exe)

		if flagEnvPath == "" {
			flagEnvPath = filepath.Join(exeDir, ".env")
		}
		if flagLogPath == "" {
			flagLogPath = filepath.Join(exeDir, "degoo-cli.log")
		}

		var err error
		log, err = logger.New(flagLogPath)
		if err != nil {
			return fmt.Errorf("open log: %w", err)
		}

		// Load .env; warn on errors other than "file not found"
		if err := godotenv.Load(flagEnvPath); err != nil && !os.IsNotExist(err) {
			log.Warn("Could not load .env file %s: %v", flagEnvPath, err)
		}

		// Try cached refresh token to get a fresh access token.
		// JWTs expire in ~1h; always refreshing avoids stale-token failures.
		cache, cacheErr := auth.LoadTokenCache()
		if cacheErr == nil && cache.RefreshToken != "" {
			newToken, refreshErr := auth.RefreshAccessToken(cache.RefreshToken)
			if refreshErr == nil {
				cache.AccessToken = newToken
				_ = auth.SaveTokenCache(cache)
				refreshToken := cache.RefreshToken
				apiClient = api.NewClient(newToken).WithRefresher(func() (string, error) {
					tok, err := auth.RefreshAccessToken(refreshToken)
					if err != nil {
						return "", err
					}
					_ = auth.SaveTokenCache(&auth.TokenCache{AccessToken: tok, RefreshToken: refreshToken})
					return tok, nil
				})
				return nil
			}
			log.Warn("Token refresh failed (%v), re-logging in", refreshErr)
		}

		// Login with credentials
		creds, err := auth.ResolveCredentials(flagEnvPath)
		if err != nil {
			return fmt.Errorf("resolve credentials: %w", err)
		}
		tokens, err := auth.Login(creds)
		if err != nil {
			return fmt.Errorf("login: %w", err)
		}
		if err := auth.SaveTokenCache(tokens); err != nil {
			log.Warn("Could not save token cache: %v", err)
		}
		refreshToken := tokens.RefreshToken
		apiClient = api.NewClient(tokens.AccessToken).WithRefresher(func() (string, error) {
			tok, err := auth.RefreshAccessToken(refreshToken)
			if err != nil {
				return "", err
			}
			_ = auth.SaveTokenCache(&auth.TokenCache{AccessToken: tok, RefreshToken: refreshToken})
			return tok, nil
		})
		log.Info("Logged in as %s", creds.Email)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if log != nil {
			return log.Close()
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagEnvPath, "env", "", "path to .env file (default: .env beside binary)")
	rootCmd.PersistentFlags().StringVar(&flagLogPath, "log", "", "path to log file (default: degoo-cli.log beside binary)")
}
