package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// runRoute parses --from/--to, resolves the stored API key via load, and
// searches for a route via findRoute. It is split out from main so it can
// be exercised by tests with fake load/findRoute functions.
func runRoute(
	args []string,
	stdout, stderr io.Writer,
	resolve providerResolver,
	geocoderConfigured func() bool,
) int {
	// Whether the caller wants JSON has to be known before Parse runs: a parse
	// failure is itself a result --json must report as a document, and Parse
	// writes usage text to the FlagSet's output on its way to returning the
	// error. Scanning args up front (as main.go already does for --debug) is
	// what lets that output be suppressed in time.
	jsonOut := hasJSONFlag(args)

	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if jsonOut {
		// stdout must hold exactly one JSON document and nothing else; usage
		// text on stderr would not corrupt it, but it would leave the caller
		// with noise it did not ask for in a mode that promises machine output.
		fs.SetOutput(io.Discard)
	}
	from := fs.String("from", "", "출발지 (역/정류장 이름 또는 주소)")
	to := fs.String("to", "", "도착지 (역/정류장 이름 또는 주소)")
	debug := fs.Bool("debug", false, "실패 시 원본 HTTP 상태·에러 체인을 함께 출력합니다")
	fs.Bool("json", false, "결과를 JSON 문서 하나로 출력합니다 (성공·실패 모두 표준 출력)")
	if err := fs.Parse(args); err != nil {
		return reportInvalidArguments(stdout, stderr, jsonOut, "명령 인자를 해석할 수 없습니다")
	}
	if *from == "" || *to == "" {
		if jsonOut {
			return reportInvalidArguments(stdout, stderr, jsonOut, "--from과 --to를 모두 입력해야 합니다")
		}
		if _, err := fmt.Fprintln(stderr, "naeryeo route: --from과 --to를 모두 입력해야 합니다"); err != nil {
			return 1
		}
		return 1
	}

	// Everything that can be known before a search — which provider, whether
	// its credential is available, whether the keychain can even be read — is
	// settled here, so those failures reach the caller through the same coded
	// document as a failed search rather than as a special case.
	findRoute, preflight := resolve()
	if preflight != nil {
		return reportFailure(stdout, stderr, jsonOut, *preflight)
	}

	result, err := findRoute(context.Background(), *from, *to)
	if err != nil {
		return reportRouteFailure(stdout, stderr, jsonOut, err, geocoderConfigured(), *debug)
	}

	if jsonOut {
		return writeJSONDocument(stdout, toRouteToolOutput(result))
	}
	return printRouteResult(stdout, *from, *to, result)
}

// writeJSONDocument emits out as the single document --json promises on stdout
// and returns the exit code that matches it: 1 when the envelope carries an
// error, 0 otherwise. Success and failure share the stream on purpose — a
// caller that captures only stdout (or pipes it) must not end up with an empty
// buffer and a bare exit code as its entire explanation (spec 005 FR-008).
func writeJSONDocument(stdout io.Writer, out RouteToolOutput) int {
	enc := json.NewEncoder(stdout)
	if err := enc.Encode(out); err != nil {
		return 1
	}
	if out.Error != nil {
		return 1
	}
	return 0
}

// reportFailure renders an already-classified failure in whichever mode is
// active, and always exits 1.
//
// The two modes use different streams on purpose: prose failures stay on stderr
// where they have always been, while the JSON document goes to stdout so a
// caller capturing only stdout still receives the reason (spec 005 FR-008).
func reportFailure(stdout, stderr io.Writer, jsonOut bool, f failure) int {
	if jsonOut {
		return writeJSONDocument(stdout, RouteToolOutput{Error: f.toRouteError()})
	}
	if _, err := fmt.Fprintln(stderr, "naeryeo route: "+f.Prose()); err != nil {
		return 1
	}
	return 1
}

// reportInvalidArguments reports a malformed invocation. In JSON mode it takes
// the same document shape as any other failure so a caller does not need a
// second parser for its own mistakes (spec 005 FR-015); in prose mode the
// caller has already written its own message and this only supplies the code.
func reportInvalidArguments(stdout, stderr io.Writer, jsonOut bool, message string) int {
	if !jsonOut {
		return 1
	}
	return reportFailure(stdout, stderr, jsonOut, failure{
		Code:    codeInvalidArguments,
		Message: message,
	})
}

// reportRouteFailure classifies a failed search and renders it.
//
// --debug's raw chain always goes to stderr — in JSON mode that keeps it from
// corrupting the document, and it is where the rest of the diagnostics
// (NAERYEO_LOG_LEVEL, the slog handler) already write (spec 005 FR-014).
func reportRouteFailure(stdout, stderr io.Writer, jsonOut bool, err error, geocoderConfigured, debug bool) int {
	if !jsonOut {
		// Prose keeps going through reportRouteError so its exact framing —
		// message and [debug] chain on a single Fprintln — is untouched.
		code, writeErr := reportRouteError(stderr, err, geocoderConfigured, debug)
		if writeErr != nil {
			return 1
		}
		return code
	}

	if debug {
		if _, writeErr := fmt.Fprintf(stderr, "[debug] %v\n", err); writeErr != nil {
			return 1
		}
	}
	return reportFailure(stdout, stderr, jsonOut, classifyRouteError(err, geocoderConfigured))
}

// reportRouteError writes a message describing err to stderr. It always
// returns 1, since this is only called on an already-known error path; a
// write failure while reporting that error is still surfaced via the
// returned error so the caller cannot silently ignore it. When debug is set
// (--debug), the raw wrapped error chain is appended so failures whose
// friendly message hides the underlying cause (e.g. a geocoder HTTP status)
// are still diagnosable without re-running under NAERYEO_LOG_LEVEL.
func reportRouteError(stderr io.Writer, err error, geocoderConfigured, debug bool) (int, error) {
	msg := "naeryeo route: " + routeErrorMessage(err, geocoderConfigured)
	if debug {
		msg += fmt.Sprintf("\n[debug] %v", err)
	}
	_, writeErr := fmt.Fprintln(stderr, msg)
	return 1, writeErr
}

// routeErrorMessage turns a core/config error from a route search into a
// human-readable Korean reason, with no CLI-specific framing. It is shared
// by the CLI (route.go, prefixed with "naeryeo route: ") and the MCP tool
// handler (mcp.go, used as-is) so both entry points give the same reason
// for the same failure (spec 002 FR-013, spec 003 FR-012).
//
// geocoderConfigured tells this function whether a place-search key is
// stored, so the "point not found" case can append the FR-007 hint only when
// setting up a geocoder would actually help (spec 004).
//
// Since spec 005 the classification itself lives in classifyRouteError, which
// also carries a stable error code and keeps the reason and the suggested
// action in separate fields. This function is the prose projection of that
// value and stays as the entry point for the callers that only need text.
func routeErrorMessage(err error, geocoderConfigured bool) string {
	return classifyRouteError(err, geocoderConfigured).Prose()
}

// withThousandsSeparator formats n with comma thousands separators, e.g.
// 1500 -> "1,500" (README's documented fare format).
func withThousandsSeparator(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}

	var out []byte
	for i, digit := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func sideLabel(side string) string {
	switch side {
	case "from":
		return "출발지"
	case "to":
		return "도착지"
	default:
		return "출발지와 도착지 모두"
	}
}

func printRouteResult(stdout io.Writer, from, to string, result core.RouteResult) int {
	if result.NoTravelNeeded {
		if _, err := fmt.Fprintf(stdout, "%s와(과) %s는 이동이 필요 없을 만큼 가까운 위치입니다\n", from, to); err != nil {
			return 1
		}
		return 0
	}

	if _, err := fmt.Fprintf(stdout, "%s → %s (약 %d분, 환승 %d회)\n\n", from, to, result.TotalTime, result.TransferCount); err != nil {
		return 1
	}
	for i, step := range result.Steps {
		if _, err := fmt.Fprintf(stdout, "%d. %s\n", i+1, step.Description); err != nil {
			return 1
		}
	}
	// "요금 정보 없음" rather than "요금: 0원": a provider that carries no fare
	// data must not be rendered as a free trip (spec 006 FR-010). ODsay always
	// sets FareKnown, so its output is unchanged.
	if !result.FareKnown {
		if _, err := fmt.Fprint(stdout, "\n요금 정보 없음\n"); err != nil {
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(stdout, "\n요금: %s원\n", withThousandsSeparator(result.Fare)); err != nil {
		return 1
	}
	return 0
}
