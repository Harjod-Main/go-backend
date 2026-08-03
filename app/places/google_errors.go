package places

import "errors"

var (
	ErrGooglePlacesNotConfigured = errors.New("google places api key is not configured")
)
