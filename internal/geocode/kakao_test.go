package geocode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

const keywordPath = "/v2/local/search/keyword.json"

// newKakao points a Kakao geocoder at a test server.
func newKakao(t *testing.T, handler http.HandlerFunc) *Kakao {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(keywordPath, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	k := NewKakao("test-key")
	k.BaseURL = srv.URL
	return k
}

func TestKakaoResolve_Success(t *testing.T) {
	k := newKakao(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"documents":[{"place_name":"아이디스 타워","x":"127.108","y":"37.401"}]}`)
	})

	got, err := k.Resolve(context.Background(), "아이디스 타워")
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.X != 127.108 || got.Y != 37.401 {
		t.Errorf("Resolve() = %+v, want {X:127.108 Y:37.401}", got)
	}
}

func TestKakaoResolve_MultipleDocsUsesFirst(t *testing.T) {
	k := newKakao(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"documents":[{"x":"127.1","y":"37.3"},{"x":"128.9","y":"35.1"}]}`)
	})

	got, err := k.Resolve(context.Background(), "여러 후보")
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got.X != 127.1 || got.Y != 37.3 {
		t.Errorf("Resolve() = %+v, want the first document {X:127.1 Y:37.3}", got)
	}
}

func TestKakaoResolve_ErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "zero documents maps to ErrGeocoderNotFound",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, `{"documents":[]}`)
			},
			wantErr: core.ErrGeocoderNotFound,
		},
		{
			name:    "HTTP 401 maps to ErrGeocoderAuthFailed",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			wantErr: core.ErrGeocoderAuthFailed,
		},
		{
			name:    "HTTP 403 maps to ErrGeocoderForbidden (key valid, service denied)",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
			wantErr: core.ErrGeocoderForbidden,
		},
		{
			name:    "HTTP 500 maps to ErrGeocoderUnavailable",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: core.ErrGeocoderUnavailable,
		},
		{
			name: "malformed JSON maps to ErrGeocoderUnavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, `{not valid json`)
			},
			wantErr: core.ErrGeocoderUnavailable,
		},
		{
			name: "non-numeric coordinate maps to ErrGeocoderUnavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, `{"documents":[{"x":"not-a-number","y":"37.3"}]}`)
			},
			wantErr: core.ErrGeocoderUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newKakao(t, tt.handler)
			_, err := k.Resolve(context.Background(), "질의어")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resolve() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestKakaoResolve_HTTP4xxPreservesStatusAndBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "dapi errorType/message shape",
			body:        `{"errorType":"InvalidArgument","message":"query is required"}`,
			wantCode:    "InvalidArgument",
			wantMessage: "query is required",
		},
		{
			name:        "platform code/msg shape (e.g. -10 call-frequency exceeded)",
			body:        `{"code":-10,"msg":"limit exceeded"}`,
			wantCode:    "-10",
			wantMessage: "limit exceeded",
		},
		{
			name:        "non-JSON body is surfaced verbatim as the message",
			body:        "Bad Request",
			wantCode:    "",
			wantMessage: "Bad Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newKakao(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				if _, err := fmt.Fprint(w, tt.body); err != nil {
					t.Fatalf("write body: %v", err)
				}
			})

			_, err := k.Resolve(context.Background(), "질의어")

			// A 400 must NOT be folded into the network-error bucket.
			if errors.Is(err, core.ErrGeocoderUnavailable) {
				t.Fatalf("Resolve() error = %v, should not be ErrGeocoderUnavailable for a 400", err)
			}
			var rej *core.ErrGeocoderRejected
			if !errors.As(err, &rej) {
				t.Fatalf("Resolve() error = %v, want *core.ErrGeocoderRejected", err)
			}
			if rej.Status != http.StatusBadRequest {
				t.Errorf("Status = %d, want 400", rej.Status)
			}
			if rej.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", rej.Code, tt.wantCode)
			}
			if !strings.Contains(rej.Message, tt.wantMessage) {
				t.Errorf("Message = %q, want to contain %q", rej.Message, tt.wantMessage)
			}
		})
	}
}

func TestKakaoResolve_ConnectionRefused(t *testing.T) {
	k := NewKakao("test-key")
	k.BaseURL = "http://127.0.0.1:1"

	_, err := k.Resolve(context.Background(), "질의어")
	if !errors.Is(err, core.ErrGeocoderUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrGeocoderUnavailable", err)
	}
}

func TestKakaoResolve_SendsAuthHeaderAndEncodedQuery(t *testing.T) {
	var gotAuth, gotQuery string
	k := newKakao(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("query")
		writeJSON(t, w, `{"documents":[{"x":"127.1","y":"37.3"}]}`)
	})

	if _, err := k.Resolve(context.Background(), "아이디스 타워"); err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if gotAuth != "KakaoAK test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "KakaoAK test-key")
	}
	if gotQuery != "아이디스 타워" {
		t.Errorf("decoded query param = %q, want %q", gotQuery, "아이디스 타워")
	}
}

// writeJSON writes a raw JSON body with the JSON content type.
func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprint(w, body); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
}
