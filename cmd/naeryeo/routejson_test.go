package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// decodeEnvelope parses stdout as the single JSON document --json promises and
// fails the test if anything else is mixed in. Callers that capture only stdout
// must be able to parse it in one shot (spec 005 FR-008).
func decodeEnvelope(t *testing.T, stdout string) RouteToolOutput {
	t.Helper()
	var out RouteToolOutput
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout is not a parseable JSON document: %v\nstdout = %q", err, stdout)
	}
	if dec.More() {
		t.Fatalf("stdout carries more than one JSON document: %q", stdout)
	}
	return out
}

// failingRoute builds a findRoute that always returns err.
func failingRoute(err error) func(context.Context, string, string) (core.RouteResult, error) {
	return func(context.Context, string, string) (core.RouteResult, error) {
		return core.RouteResult{}, err
	}
}

// TestRunRouteJSON_FailureGoesToStdoutWithExitOne is the core of FR-008: a
// caller that keeps only stdout must still learn why the search failed. If the
// failure document went to stderr, piping stdout would leave the caller with an
// empty buffer and a bare exit code.
func TestRunRouteJSON_FailureGoesToStdoutWithExitOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runRoute(
		[]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
		&stdout, &stderr, staticProvider(failingRoute(core.ErrNoRoute)), geoPresent,
	)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty in --json mode", stderr.String())
	}
	got := decodeEnvelope(t, stdout.String())
	if got.Error == nil {
		t.Fatal("Error is nil; a failure document must carry an error object")
	}
	if got.Error.Code != string(codeNoRoute) {
		t.Errorf("Error.Code = %q, want %q", got.Error.Code, codeNoRoute)
	}
	if got.Error.Message == "" {
		t.Error("Error.Message is empty; it must be relayable to the user as-is")
	}
	if got.TotalTimeMinutes != 0 || got.Steps != nil {
		t.Errorf("success fields leaked into a failure document: %+v", got)
	}
}

// TestRunRouteJSON_CodesDistinguishNextAction walks the branches an AI caller
// must be able to tell apart without reading a single Korean sentence.
func TestRunRouteJSON_CodesDistinguishNextAction(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		geocoderConfigured func() bool
		wantCode           errorCode
		wantSide           string
		wantName           string
		wantHint           bool
	}{
		{
			name:               "rate limit tells the caller to retry",
			err:                &core.ErrGeocoderRejected{Status: http.StatusTooManyRequests},
			geocoderConfigured: geoPresent,
			wantCode:           codeGeocoderRateLimited,
		},
		{
			name:               "kakao code -10 is also a rate limit",
			err:                &core.ErrGeocoderRejected{Status: http.StatusBadRequest, Code: "-10"},
			geocoderConfigured: geoPresent,
			wantCode:           codeGeocoderRateLimited,
		},
		{
			name:               "a plain rejection tells the caller to reformulate",
			err:                &core.ErrGeocoderRejected{Status: http.StatusBadRequest},
			geocoderConfigured: geoPresent,
			wantCode:           codeGeocoderRejected,
		},
		{
			name:               "auth failure is fixable by re-registering the key",
			err:                core.ErrGeocoderAuthFailed,
			geocoderConfigured: geoPresent,
			wantCode:           codeGeocoderAuthFailed,
		},
		{
			name:               "forbidden is NOT fixable by re-registering the key",
			err:                core.ErrGeocoderForbidden,
			geocoderConfigured: geoPresent,
			wantCode:           codeGeocoderForbidden,
		},
		{
			name:               "point not found carries the failing side and input",
			err:                &core.ErrPointNotFound{Side: "from", Name: "아이디스 타워"},
			geocoderConfigured: geoPresent,
			wantCode:           codePointNotFound,
			wantSide:           "from",
			wantName:           "아이디스 타워",
		},
		{
			name:               "point not found without a geocoder adds an actionable hint",
			err:                &core.ErrPointNotFound{Side: "to", Name: "수지구청"},
			geocoderConfigured: geoAbsent,
			wantCode:           codePointNotFound,
			wantSide:           "to",
			wantName:           "수지구청",
			wantHint:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runRoute(
				[]string{"--from", "출발", "--to", "도착", "--json"},
				&stdout, &stderr, staticProvider(failingRoute(tt.err)), tt.geocoderConfigured,
			)
			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}

			got := decodeEnvelope(t, stdout.String())
			if got.Error == nil {
				t.Fatal("Error is nil")
			}
			if got.Error.Code != string(tt.wantCode) {
				t.Errorf("Code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if got.Error.Side != tt.wantSide {
				t.Errorf("Side = %q, want %q", got.Error.Side, tt.wantSide)
			}
			if got.Error.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Error.Name, tt.wantName)
			}
			if hasHint := got.Error.Hint != ""; hasHint != tt.wantHint {
				t.Errorf("Hint present = %v, want %v (hint = %q)", hasHint, tt.wantHint, got.Error.Hint)
			}
		})
	}
}

// TestRunRouteJSON_RateLimitedAndRejectedAreDifferentCodes states the
// distinction SC-002 turns on, separately from the table above so a regression
// names itself clearly.
func TestRunRouteJSON_RateLimitedAndRejectedAreDifferentCodes(t *testing.T) {
	if codeGeocoderRateLimited == codeGeocoderRejected {
		t.Fatal("a retryable limit and a malformed request must not share a code")
	}
	if codeGeocoderAuthFailed == codeGeocoderForbidden {
		t.Fatal("a re-registerable key failure and a console-settings failure must not share a code")
	}
}

// TestRunRouteJSON_InvalidArgumentsAreStillMachineReadable covers FR-015. A
// caller that got the invocation wrong needs to learn that from the same
// document shape, not from usage text on stderr.
func TestRunRouteJSON_InvalidArgumentsAreStillMachineReadable(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing both endpoints", args: []string{"--json"}},
		{name: "missing --to", args: []string{"--from", "강남역", "--json"}},
		{name: "unknown flag", args: []string{"--from", "강남역", "--to", "홍대", "--nope", "--json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			findRoute := func(context.Context, string, string) (core.RouteResult, error) {
				called = true
				return core.RouteResult{}, nil
			}

			var stdout, stderr bytes.Buffer
			code := runRoute(tt.args, &stdout, &stderr, staticProvider(findRoute), geoPresent)

			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			if called {
				t.Error("findRoute was called despite invalid arguments")
			}
			got := decodeEnvelope(t, stdout.String())
			if got.Error == nil {
				t.Fatal("Error is nil")
			}
			if got.Error.Code != string(codeInvalidArguments) {
				t.Errorf("Code = %q, want %q", got.Error.Code, codeInvalidArguments)
			}
		})
	}
}

// TestRunRouteJSON_DebugDoesNotCorruptTheDocument covers FR-014: --debug adds
// diagnostics, --json selects a format, and combining them must not put the raw
// error chain where a parser will choke on it.
func TestRunRouteJSON_DebugDoesNotCorruptTheDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runRoute(
		[]string{"--from", "강남역", "--to", "홍대입구역", "--json", "--debug"},
		&stdout, &stderr, staticProvider(failingRoute(core.ErrNoRoute)), geoPresent,
	)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	got := decodeEnvelope(t, stdout.String())
	if got.Error == nil || got.Error.Code != string(codeNoRoute) {
		t.Fatalf("stdout document = %+v, want a no_route failure", got)
	}
	if !strings.Contains(stderr.String(), "[debug]") {
		t.Errorf("stderr = %q, want the debug chain to go here", stderr.String())
	}
}

// TestRunRouteJSON_Success covers the success half of the envelope: the fields
// an AI caller consumes without parsing prose, and the absence of "error" as
// the sole success signal.
func TestRunRouteJSON_Success(t *testing.T) {
	findRoute := func(context.Context, string, string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime:     42,
			TransferCount: 1,
			Fare:          1500,
			FareKnown:     true,
			Steps: []core.RouteStep{
				{Description: "강남역에서 2호선 승차 → 신도림역에서 하차"},
				{Description: "신도림역에서 경의중앙선 승차 → 홍대입구역에서 하차"},
			},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute(
		[]string{"--from", "강남역", "--to", "홍대입구역", "--json"},
		&stdout, &stderr, staticProvider(findRoute), geoPresent,
	)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr.String())
	}
	got := decodeEnvelope(t, stdout.String())
	if got.Error != nil {
		t.Fatalf("Error = %+v, want nil on success", got.Error)
	}
	if got.TotalTimeMinutes != 42 || got.TransferCount != 1 {
		t.Errorf("got %+v, want 42min / 1 transfer", got)
	}
	if got.FareWon == nil || *got.FareWon != 1500 {
		t.Errorf("FareWon = %v, want a pointer to 1500", got.FareWon)
	}
	if len(got.Steps) != 2 || !strings.Contains(got.Steps[0], "강남역") {
		t.Errorf("Steps = %v, want the two step descriptions in order", got.Steps)
	}
	// "error" must be absent, not present-and-null: callers key off the field.
	if bytes.Contains(stdout.Bytes(), []byte(`"error"`)) {
		t.Errorf("success document contains an error key: %s", stdout.String())
	}
}

// TestRunRouteJSON_ZeroValuedSuccessFieldsAreOmitted pins the presence
// semantics of the success document, which are no longer uniform across the
// numeric fields.
//
// transferCount keeps the original rule — it is a plain int with omitempty, so
// an absent field means zero. Emitting zeros on success is still not an option:
// the envelope is one type, so it would stamp "totalTimeMinutes":0 onto every
// failure document too.
//
// fareWon is different since spec 006 FR-010. It is a *int, so absence means
// "the provider gave no fare information" and a present 0 means "this trip is
// free". Collapsing those two into one absent field is exactly the ambiguity a
// self-hosted engine without fare data would otherwise land in — it would be
// reported as free. See TestRunRouteJSON_KnownZeroFareIsEmitted below for the
// other half of this contract.
func TestRunRouteJSON_ZeroValuedSuccessFieldsAreOmitted(t *testing.T) {
	findRoute := func(context.Context, string, string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime:     18,
			TransferCount: 0,
			// FareKnown is deliberately false: this fixture is a provider that
			// supplied no fare at all.
			Steps: []core.RouteStep{{Description: "강남역에서 2호선 승차 → 역삼역에서 하차"}},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	if code := runRoute(
		[]string{"--from", "강남역", "--to", "역삼역", "--json"},
		&stdout, &stderr, staticProvider(findRoute), geoPresent,
	); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	raw := stdout.String()
	for _, absent := range []string{`"transferCount"`, `"fareWon"`} {
		if strings.Contains(raw, absent) {
			t.Errorf("expected %s to be omitted, got: %s", absent, raw)
		}
	}

	got := decodeEnvelope(t, raw)
	if got.TransferCount != 0 {
		t.Errorf("decoded transferCount = %d, want 0", got.TransferCount)
	}
	if got.FareWon != nil {
		t.Errorf("decoded fareWon = %d, want nil — an absent fare is unknown, not free", *got.FareWon)
	}
	if got.TotalTimeMinutes != 18 {
		t.Errorf("TotalTimeMinutes = %d, want 18", got.TotalTimeMinutes)
	}
}

// TestRunRouteJSON_KnownZeroFareIsEmitted is the counterpart: when a provider
// does report a fare and that fare happens to be zero, the field is present
// with value 0 rather than dropping out. ODsay always reports a fare, so this
// is the shape every ODsay result takes.
func TestRunRouteJSON_KnownZeroFareIsEmitted(t *testing.T) {
	findRoute := func(context.Context, string, string) (core.RouteResult, error) {
		return core.RouteResult{
			TotalTime: 18,
			Fare:      0,
			FareKnown: true,
			Steps:     []core.RouteStep{{Description: "강남역에서 2호선 승차 → 역삼역에서 하차"}},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	if code := runRoute(
		[]string{"--from", "강남역", "--to", "역삼역", "--json"},
		&stdout, &stderr, staticProvider(findRoute), geoPresent,
	); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	raw := stdout.String()
	if !strings.Contains(raw, `"fareWon":0`) {
		t.Errorf("expected a known zero fare to be emitted as \"fareWon\":0, got: %s", raw)
	}

	got := decodeEnvelope(t, raw)
	if got.FareWon == nil || *got.FareWon != 0 {
		t.Errorf("decoded fareWon = %v, want a pointer to 0", got.FareWon)
	}
}

// TestRunRouteJSON_NoTravelNeeded keeps "you are already there" from being read
// as "the search returned nothing": a zero duration alone is ambiguous, the
// explicit flag is not.
func TestRunRouteJSON_NoTravelNeeded(t *testing.T) {
	findRoute := func(context.Context, string, string) (core.RouteResult, error) {
		return core.RouteResult{NoTravelNeeded: true}, nil
	}

	var stdout, stderr bytes.Buffer
	code := runRoute(
		[]string{"--from", "강남역", "--to", "강남역 2번 출구", "--json"},
		&stdout, &stderr, staticProvider(findRoute), geoPresent,
	)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	got := decodeEnvelope(t, stdout.String())
	if !got.NoTravelNeeded {
		t.Errorf("NoTravelNeeded = false, want true; document = %s", stdout.String())
	}
	if got.Error != nil {
		t.Errorf("Error = %+v, want nil", got.Error)
	}
}

// TestRunRouteJSON_SuccessMatchesMCP is FR-010's automatic check. Type identity
// already makes drift impossible, but this pins the guarantee so that splitting
// the envelope back into two types fails loudly rather than quietly
// reintroducing two schemas for one payload.
//
// The comparison is on the decoded documents, not on raw bytes: the MCP side
// arrives as a decoded map whose keys re-marshal in alphabetical order, while
// the CLI side marshals the struct in field order. Key ordering is not part of
// the contract — the set of keys and their values is.
func TestRunRouteJSON_SuccessMatchesMCP(t *testing.T) {
	results := []core.RouteResult{
		{
			TotalTime:     42,
			TransferCount: 1,
			Fare:          1500,
			FareKnown:     true,
			Steps:         []core.RouteStep{{Description: "강남역에서 2호선 승차"}},
		},
		{NoTravelNeeded: true},
	}

	for i, result := range results {
		findRoute := func(context.Context, string, string) (core.RouteResult, error) {
			return result, nil
		}

		var stdout, stderr bytes.Buffer
		if code := runRoute(
			[]string{"--from", "출발", "--to", "도착", "--json"},
			&stdout, &stderr, staticProvider(findRoute), geoPresent,
		); code != 0 {
			t.Fatalf("result %d: CLI code = %d, want 0", i, code)
		}

		server := buildMCPServer("test", discardLogger, staticProvider(findRoute), geoPresent)
		session := connectTestClient(t, server)
		res := callFindTransitRoute(t, session, "출발", "도착")

		mcpJSON, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("result %d: marshal StructuredContent: %v", i, err)
		}

		var cliDoc, mcpDoc map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &cliDoc); err != nil {
			t.Fatalf("result %d: decode CLI document: %v", i, err)
		}
		if err := json.Unmarshal(mcpJSON, &mcpDoc); err != nil {
			t.Fatalf("result %d: decode MCP document: %v", i, err)
		}

		if !reflect.DeepEqual(cliDoc, mcpDoc) {
			t.Errorf("result %d: success schema drift\nCLI = %v\nMCP = %v", i, cliDoc, mcpDoc)
		}
	}
}

// TestRunRoute_ProseModeUnchangedByJSONSupport guards SC-007 from the other
// direction: adding --json must not have altered the default path's streams.
func TestRunRoute_ProseModeUnchangedByJSONSupport(t *testing.T) {
	t.Run("failure still goes to stderr", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runRoute(
			[]string{"--from", "강남역", "--to", "홍대입구역"},
			&stdout, &stderr, staticProvider(failingRoute(core.ErrNoRoute)), geoPresent,
		)
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want empty on the prose failure path", stdout.String())
		}
		if !strings.Contains(stderr.String(), "naeryeo route: ") {
			t.Errorf("stderr = %q, want the CLI-framed prose message", stderr.String())
		}
	})

	t.Run("prose output is not JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		runRoute(
			[]string{"--from", "강남역", "--to", "홍대입구역"},
			&stdout, &stderr, staticProvider(func(context.Context, string, string) (core.RouteResult, error) {
				return core.RouteResult{TotalTime: 42, TransferCount: 1, Fare: 1500, FareKnown: true}, nil
			}), geoPresent,
		)
		var probe any
		if err := json.Unmarshal(stdout.Bytes(), &probe); err == nil {
			t.Errorf("default output parsed as JSON; prose mode must be unchanged: %q", stdout.String())
		}
	})
}
