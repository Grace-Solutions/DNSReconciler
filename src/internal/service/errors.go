package service

import "errors"

// ErrUnsupported is returned when the current platform has no service manager implementation.
var ErrUnsupported = errors.New("service management is not supported on this platform")