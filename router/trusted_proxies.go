package router

import (
	"fmt"
	"strings"

	"github.com/RinTanth/go-backend/config"
	"github.com/gin-gonic/gin"
)

// defaultLocalProxyCIDRs is used only for ENV=LOCAL (direct dev server / emulator).
var defaultLocalProxyCIDRs = []string{
	"127.0.0.1/32",
	"::1/128",
}

func applyTrustedProxies(r *gin.Engine, cfg config.Config) error {
	cidrs := trustedProxyCIDRs(cfg)
	if err := r.SetTrustedProxies(cidrs); err != nil {
		return fmt.Errorf("set trusted proxies: %w", err)
	}
	// Gin defaults to trusting 0.0.0.0/0 — we replace that with explicit CIDRs only.
	r.ForwardedByClientIP = true
	r.RemoteIPHeaders = []string{"X-Forwarded-For", "X-Real-IP"}
	return nil
}

func trustedProxyCIDRs(cfg config.Config) []string {
	if raw := strings.TrimSpace(cfg.Server.TrustedProxyCIDRs); raw != "" {
		return splitCSV(raw)
	}
	if config.IsLocalEnv() {
		return append([]string(nil), defaultLocalProxyCIDRs...)
	}
	// Non-local deploys must set TRUSTED_PROXY_CIDRS to the LB ingress CIDR(s).
	// Do not default to broad RFC1918 — shared NAT/LB ranges allow X-Forwarded-For spoofing.
	return nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
