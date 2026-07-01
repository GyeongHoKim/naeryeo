// Package core wraps the ODsay API to find transit routes between two named
// locations. Client.FindRoute is the shared entry point for both the
// naeryeo CLI and MCP server: it resolves From/To names to coordinates,
// searches for a route, and maps the result (or failure) onto RouteResult
// and this package's sentinel errors. It takes the ODsay API key as a
// plain string rather than depending on internal/config, keeping the two
// packages loosely coupled.
package core
