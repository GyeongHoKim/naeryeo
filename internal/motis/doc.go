// Package motis adapts a self-hosted MOTIS routing engine to this project's
// domain model. MOTIS is an open-source multimodal routing engine the user
// runs on their own hardware, so naeryeo works without an account with — or
// the pricing policy of — any commercial routing provider.
//
// It depends on internal/core in one direction only: it consumes core's
// RouteResult, Coordinate, Geocoder, and error contract, and core does not
// import this package. That is the same shape internal/geocode already uses,
// and it is what lets the cmd layer choose between this client and core's
// ODsay client without either knowing about the other.
//
// Two MOTIS endpoints are used. /api/v6/plan accepts only coordinates or stop
// IDs — never a free-form name — so /api/v1/geocode does the name resolution
// that ODsay's searchStation does for the other provider. A core.Geocoder
// (Kakao) may additionally be supplied as a fallback for building names and
// addresses MOTIS does not index; that axis is optional and independent of
// the routing provider, exactly as it is for ODsay.
package motis
