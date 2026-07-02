// Package motis is the routing backend client for naeryeo's cloud track
// (the PlayMCP remote MCP server). It talks to a self-hosted MOTIS server
// (https://github.com/motis-project/motis) that serves nationwide Korean
// transit data (KTDB GTFS + OSM), and maps MOTIS responses onto the shared
// core domain model (core.RouteResult).
//
// The local CLI/stdio track keeps using internal/core's ODsay client with
// the user's own API key; this package never touches ODsay or the OS
// keychain. The two backends stay swappable because both are consumed
// through the same closure shape in cmd/naeryeo:
//
//	func(ctx context.Context, from, to string) (core.RouteResult, error)
//
// The consumed MOTIS HTTP API (two endpoints only) is documented with
// measured responses in specs/005-playmcp-cloud-server/contracts/motis-api.md.
package motis
