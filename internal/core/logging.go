package core

import (
	"errors"
	"log/slog"
	"net/url"
)

// redactURL returns rawURL with the "apiKey" query parameter's value
// replaced, safe for inclusion in logs. If rawURL cannot be parsed, a fixed
// placeholder is returned instead of the raw string, so a malformed URL
// never accidentally leaks a key into logs.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "REDACTED (unparseable URL)"
	}
	q := u.Query()
	if q.Has("apiKey") {
		q.Set("apiKey", "REDACTED")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// logger returns c.Logger, or a discard logger if unset. Mirrors the
// existing httpClient()/baseURL() nil-defaulting helpers.
func (c *Client) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// routeOutcome classifies a FindRoute result into a short, log-friendly
// tag. It never includes the API key — no error value in this package
// carries it. errNoTravelNeeded never reaches here as err (FindRoute turns
// it into result.NoTravelNeeded before returning), so that case is handled
// by the caller inspecting the result directly, not by this function.
func routeOutcome(result RouteResult, err error) string {
	switch {
	case err == nil && result.NoTravelNeeded:
		return "no_travel_needed"
	case err == nil:
		return "success"
	}

	var pointErr *ErrPointNotFound
	var rejectedErr *ErrUpstreamRejected
	switch {
	case errors.Is(err, ErrAPIKeyMissing):
		return "api_key_missing"
	case errors.Is(err, ErrAuthFailed):
		return "auth_failed"
	case errors.Is(err, ErrNoRoute):
		return "no_route"
	case errors.Is(err, ErrGeocoderAuthFailed):
		return "geocoder_auth_failed"
	case errors.Is(err, ErrGeocoderUnavailable):
		return "geocoder_unavailable"
	case errors.Is(err, ErrUpstreamUnavailable):
		return "upstream_unavailable"
	case errors.As(err, &pointErr):
		return "point_not_found:" + pointErr.Side
	case errors.As(err, &rejectedErr):
		return "upstream_rejected"
	default:
		return "unknown"
	}
}
