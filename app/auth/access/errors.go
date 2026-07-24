package access

import "errors"

// ErrUserNotFound is returned when GetUserByEmail finds no matching row.
var ErrUserNotFound = errors.New("user not found")
