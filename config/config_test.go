package config

import (
	"testing"
)

func TestPrefixEnv(t *testing.T) {
	t.Run("Prefix is empty", func(t *testing.T) {
		prefix := prefix("")

		if prefix != "" {
			t.Error("Expected empty string but got", prefix)
		}
	})

	t.Run("Prefix is not empty", func(t *testing.T) {
		prefix := prefix("LOCAL")

		if prefix != "LOCAL_" {
			t.Error("Expected LOCAL_ but got", prefix)
		}
	})
}

func TestValidateFoundation_ProdCORS(t *testing.T) {
	prev := Env
	t.Cleanup(func() { Env = prev })

	base := Config{
		Supabase: Supabase{ProjectURL: "https://example.supabase.co"},
	}

	t.Run("PROD rejects star", func(t *testing.T) {
		Env = Prod
		cfg := base
		cfg.AccessControl.AllowOrigin = "*"
		if err := validateFoundation(cfg); err == nil {
			t.Fatal("expected error for ACCESS_CONTROL_ALLOW_ORIGIN=* in PROD")
		}
	})

	t.Run("PROD rejects missing trusted proxy CIDRs", func(t *testing.T) {
		Env = Prod
		cfg := base
		cfg.AccessControl.AllowOrigin = "https://frontend-statio-s-projects.vercel.app"
		if err := validateFoundation(cfg); err == nil {
			t.Fatal("expected error when TRUSTED_PROXY_CIDRS is empty in PROD")
		}
	})

	t.Run("PROD rejects ENABLE_DEBUG_CLIENT_IP", func(t *testing.T) {
		Env = Prod
		cfg := base
		cfg.AccessControl.AllowOrigin = "https://frontend-statio-s-projects.vercel.app"
		cfg.Server.TrustedProxyCIDRs = "10.0.0.1/32"
		cfg.Server.EnableDebugClientIP = true
		if err := validateFoundation(cfg); err == nil {
			t.Fatal("expected error when ENABLE_DEBUG_CLIENT_IP is true in PROD")
		}
	})

	t.Run("PROD accepts explicit origin and trusted proxy CIDRs", func(t *testing.T) {
		Env = Prod
		cfg := base
		cfg.AccessControl.AllowOrigin = "https://frontend-statio-s-projects.vercel.app"
		cfg.Server.TrustedProxyCIDRs = "10.0.0.1/32"
		if err := validateFoundation(cfg); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LOCAL allows star", func(t *testing.T) {
		Env = Local
		cfg := base
		cfg.AccessControl.AllowOrigin = "*"
		if err := validateFoundation(cfg); err != nil {
			t.Fatal(err)
		}
	})
}
