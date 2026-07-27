package config

import (
	"log/slog"

	env "github.com/caarlos0/env/v11"
)

func parseEnv[T any](opts env.Options) (T, error) {
	var base T

	// Pass 1: parse without prefix so non-prefixed env vars (e.g. PORT)
	// act as the base/default values.
	if err := env.Parse(&base); err != nil {
		return base, err
	}

	if opts.Prefix == "" {
		return base, nil
	}

	// Pass 2: re-parse into a copy so PREFIX_XXX can override. On failure
	// (common when PROD_* vars are unset on PaaS), keep the pass-1 values —
	// ParseWithOptions may partially mutate the target with envDefault before
	// returning notEmpty errors.
	override := base
	if err := env.ParseWithOptions(&override, opts); err != nil {
		slog.Warn("parseEnv: prefixed override failed, falling back to base value", "prefix", opts.Prefix, "error", err)
		return base, nil
	}

	return override, nil
}
