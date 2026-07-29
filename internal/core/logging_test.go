package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "apiKey is replaced",
			in:   "https://api.odsay.com/v1/api/searchStation?apiKey=super-secret-key&stationName=%EA%B0%95%EB%82%A8%EC%97%AD",
			want: "REDACTED",
		},
		{
			name: "no apiKey param is left untouched",
			in:   "https://api.odsay.com/v1/api/searchStation?stationName=x",
			want: "stationName=x",
		},
		{
			name: "unparseable URL never returns the raw input",
			in:   "://not a url",
			want: "REDACTED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURL(tt.in)
			if !strings.Contains(got, tt.want) {
				t.Errorf("redactURL(%q) = %q, want substring %q", tt.in, got, tt.want)
			}
			if strings.Contains(got, "super-secret-key") {
				t.Errorf("redactURL(%q) = %q, leaked the API key", tt.in, got)
			}
		})
	}
}

func TestRedactURLError(t *testing.T) {
	const secret = "super-secret-key"
	rawURL := "https://api.odsay.com/v1/api/searchStation?apiKey=" + secret + "&stationName=x"

	t.Run("a *url.Error keeps its op and cause but loses the key", func(t *testing.T) {
		cause := errors.New("connection refused")
		got := redactURLError(&url.Error{Op: "Get", URL: rawURL, Err: cause})

		if strings.Contains(got.Error(), secret) {
			t.Errorf("redactURLError() = %q, leaked the API key", got)
		}
		for _, want := range []string{"Get", "REDACTED", "searchStation", "connection refused"} {
			if !strings.Contains(got.Error(), want) {
				t.Errorf("redactURLError() = %q, want substring %q", got, want)
			}
		}
		if !errors.Is(got, cause) {
			t.Errorf("redactURLError() = %q, want it to still unwrap to the cause", got)
		}
	})

	t.Run("an unparseable URL is dropped whole rather than partially redacted", func(t *testing.T) {
		got := redactURLError(&url.Error{Op: "parse", URL: "://" + secret, Err: errors.New("missing protocol scheme")})

		if strings.Contains(got.Error(), secret) {
			t.Errorf("redactURLError() = %q, leaked the API key", got)
		}
	})

	t.Run("an error carrying no URL is returned unchanged", func(t *testing.T) {
		cause := errors.New("some other failure")
		if got := redactURLError(cause); got != cause {
			t.Errorf("redactURLError() = %v, want the input error itself", got)
		}
	})
}

// TestFindRoute_NeverLogsTheAPIKey is the security-critical proof that no
// log line produced by doGet/resolveStation/FindRoute contains the API key,
// across the whole HTTP path.
func TestFindRoute_NeverLogsTheAPIKey(t *testing.T) {
	const secret = "super-secret-key"

	station := stationHandler(t, map[string]stationCandidate{
		"강남역":   {Name: "강남역", X: 127.0, Y: 37.5},
		"홍대입구역": {Name: "홍대입구역", X: 126.9, Y: 37.5},
	})
	path := func(w http.ResponseWriter, r *http.Request) {
		resp := pathSearchResponse{Result: &struct {
			Path []pathCandidate `json:"path"`
		}{Path: []pathCandidate{{}}}}
		writeJSON(t, w, resp)
	}
	srv := newTestServer(t, station, path)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := &Client{APIKey: secret, BaseURL: srv.URL, Logger: logger}

	if _, err := c.FindRoute(context.Background(), "강남역", "홍대입구역"); err != nil {
		t.Fatalf("FindRoute() error = %v, want nil", err)
	}

	if strings.Contains(buf.String(), secret) {
		t.Fatalf("log output contains the API key value: %q", buf.String())
	}
	if buf.Len() == 0 {
		t.Fatal("expected some log output, got none")
	}
}

// TestClassifyODsayError_LogsRawErrorDetails proves the raw ODsay code and
// message are captured before being mapped to a domain error — the
// highest-value line for diagnosing unconfirmed ODsay behavior.
func TestClassifyODsayError_LogsRawErrorDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	c := &Client{Logger: logger}

	err := c.classifyODsayError(&odsayErrorBody{Code: "-9", Message: "missing required param"}, "강남역", "홍대입구역")
	if err == nil {
		t.Fatal("classifyODsayError() error = nil, want non-nil")
	}

	var record map[string]any
	if decErr := json.Unmarshal(buf.Bytes(), &record); decErr != nil {
		t.Fatalf("log output is not valid JSON: %v; output = %q", decErr, buf.String())
	}
	if record["code"] != "-9" {
		t.Errorf("logged code = %v, want %q", record["code"], "-9")
	}
	if record["message"] != "missing required param" {
		t.Errorf("logged message = %v, want %q", record["message"], "missing required param")
	}
}
