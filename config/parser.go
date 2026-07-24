package config

import (
	"log/slog"

	env "github.com/caarlos0/env/v11"
)

func parseEnv[T any](opts env.Options) (T, error) {
	var t T

	// Pass 1: parse without prefix so non-prefixed env vars (e.g. ENDPOINT_URL)
	// act as the base/default values.
	if err := env.Parse(&t); err != nil {
		return t, err
	}

	// Pass 2: re-parse with the prefix so PREFIX_XXX_XXX overrides the base
	// value when set. Errors here are logged rather than returned: a missing
	// prefixed var is expected (falls back to the pass-1 value), but a
	// malformed one (e.g. bad type conversion) would otherwise fail silently,
	// so surface it instead of swallowing it outright.
	if err := env.ParseWithOptions(&t, opts); err != nil {
		slog.Warn("parseEnv: prefixed override failed, falling back to base value", "prefix", opts.Prefix, "error", err)
	}

	return t, nil
}
