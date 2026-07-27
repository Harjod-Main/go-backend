package router

import (
	"fmt"
	"strings"

	"github.com/RinTanth/go-backend/config"
	"github.com/gin-gonic/gin"
)

// defaultPrivateProxyCIDRs covers typical PaaS load balancers (Render, Fly, etc.)
// that connect from RFC1918 / link-local addresses and set X-Forwarded-For.
var defaultPrivateProxyCIDRs = []string{
	"127.0.0.1/32",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

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
	return append([]string(nil), defaultPrivateProxyCIDRs...)
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
