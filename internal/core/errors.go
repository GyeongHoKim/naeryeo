package core

import (
	"errors"
	"fmt"
)

var (
	// ErrAPIKeyMissing indicates FindRoute was called with an empty API key.
	// No network call is made when this is returned.
	ErrAPIKeyMissing = errors.New("core: ODsay API key is not set")
	// ErrAuthFailed indicates ODsay rejected the request as unauthenticated
	// (the stored API key is invalid or expired).
	ErrAuthFailed = errors.New("core: ODsay rejected the API key")
	// ErrNoRoute indicates the two points are recognized but no transit
	// route connects them.
	ErrNoRoute = errors.New("core: no transit route between the two points")
	// ErrUpstreamUnavailable indicates a network error, timeout, or ODsay
	// server-side failure.
	ErrUpstreamUnavailable = errors.New("core: ODsay is unavailable")
)

// ErrPointNotFound indicates the from and/or to location name could not be
// resolved to a real station or stop.
type ErrPointNotFound struct {
	// Side is "from", "to", or "both".
	Side string
	Name string
}

func (e *ErrPointNotFound) Error() string {
	return fmt.Sprintf("core: could not recognize %s location %q", e.Side, e.Name)
}

// ErrUpstreamRejected indicates ODsay returned an error code that doesn't
// map to any of the other sentinel errors above.
type ErrUpstreamRejected struct {
	Code    string
	Message string
}

func (e *ErrUpstreamRejected) Error() string {
	return fmt.Sprintf("core: ODsay rejected the request (code %s): %s", e.Code, e.Message)
}
