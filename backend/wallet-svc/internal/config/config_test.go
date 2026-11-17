package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_SECRET", "")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "wallet-svc", cfg.App.Name)
	require.NotEmpty(t, cfg.Auth.Secrets)
	require.Equal(t, defaultDevJWTSecret, cfg.Auth.Secrets[0])
}

func TestProductionRequiresToken(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	require.Error(t, err)
}
