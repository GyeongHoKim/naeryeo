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
	// ErrGeocoderAuthFailed indicates the geocoder rejected the place-search
	// API key as unauthenticated (HTTP 401) — the key is missing, wrong, or
	// expired, so re-registering it may help. Kept distinct from
	// ErrPointNotFound so callers can tell "the key is invalid" apart from
	// "the name matched nothing".
	ErrGeocoderAuthFailed = errors.New("core: geocoder rejected the API key")
	// ErrGeocoderForbidden indicates the geocoder accepted the key but denied
	// the request (HTTP 403) — e.g. the required map/local service is not
	// enabled for the app, or a domain/IP restriction applies. Re-registering
	// the same key will NOT help; the fix is in the provider's app settings,
	// so this is kept distinct from ErrGeocoderAuthFailed.
	ErrGeocoderForbidden = errors.New("core: geocoder denied the request")
	// ErrGeocoderNotFound is the not-found signal a Geocoder returns when a
	// name matches no place. resolveStation folds it into the same
	// "unrecognized point" handling as a failed station search.
	//
	// ErrGeocoderNotFound, ErrGeocoderAuthFailed, and ErrGeocoderUnavailable
	// make up the error contract of the Geocoder interface. They live here,
	// in the consuming package, alongside the interface and Coordinate type,
	// so a Geocoder implementation depends only on core (one-way) and core
	// can classify its result via errors.Is without importing the
	// implementation — avoiding an import cycle.
	ErrGeocoderNotFound = errors.New("core: geocoder found no matching place")
	// ErrGeocoderUnavailable indicates a network error, timeout, non-2xx
	// status, or unparseable response from the geocoder.
	ErrGeocoderUnavailable = errors.New("core: geocoder is unavailable")
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
