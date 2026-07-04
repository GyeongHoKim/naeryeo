// Package tmap is a cloud-track routing backend client, an alternative to
// internal/motis for the PlayMCP remote MCP server. It talks to SK Open
// API's TMAP products (POI search + 대중교통 길찾기) using a single shared
// appKey, and maps responses onto the shared core domain model
// (core.RouteResult).
//
// The local CLI/stdio track keeps using internal/core's ODsay client with
// the user's own API key; this package never touches ODsay or the OS
// keychain. All three cloud-eligible backends (motis, tmap, core/ODsay)
// stay swappable because each is consumed through the same closure shape
// in cmd/naeryeo:
//
//	func(ctx context.Context, from, to string) (core.RouteResult, error)
//
// Unlike MOTIS, TMAP's transit route endpoint takes coordinates only, so
// this package does its own name→coordinate resolution via TMAP's POI
// search rather than reusing internal/geocode (Kakao) — one appKey covers
// both, which keeps the cloud deployment's secret count down.
package tmap
