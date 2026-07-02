// Package geocode resolves free-form place names — building names, road/lot
// addresses, and other points of interest that are not registered transit
// stops — into coordinates, using the Kakao Local keyword-search API. It
// exists so internal/core can fall back to a general geocoder when ODsay's
// station search fails to recognize a From/To name. The Kakao client
// satisfies core's consumer-defined Geocoder interface; core does not import
// this package, keeping the two loosely coupled (the cmd layer wires them
// together).
package geocode
