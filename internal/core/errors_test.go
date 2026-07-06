package core

import (
	"net/http"
	"testing"
)

func TestErrGeocoderRejected_Error(t *testing.T) {
	tests := []struct {
		name string
		err  ErrGeocoderRejected
		want string
	}{
		{
			name: "code and message",
			err:  ErrGeocoderRejected{Status: 400, Code: "-10", Message: "call frequency exceeded"},
			want: "core: geocoder rejected the request (HTTP 400, code -10): call frequency exceeded",
		},
		{
			name: "message only",
			err:  ErrGeocoderRejected{Status: 400, Message: "bad request"},
			want: "core: geocoder rejected the request (HTTP 400): bad request",
		},
		{
			name: "code only is not dropped",
			err:  ErrGeocoderRejected{Status: 400, Code: "-10"},
			want: "core: geocoder rejected the request (HTTP 400, code -10)",
		},
		{
			name: "status only",
			err:  ErrGeocoderRejected{Status: 400},
			want: "core: geocoder rejected the request (HTTP 400)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrGeocoderRejected_RateLimited(t *testing.T) {
	tests := []struct {
		name string
		err  ErrGeocoderRejected
		want bool
	}{
		{"kakao code -10 is a call-frequency limit", ErrGeocoderRejected{Status: http.StatusBadRequest, Code: "-10"}, true},
		{"HTTP 429 is a rate limit", ErrGeocoderRejected{Status: http.StatusTooManyRequests}, true},
		{"invalid-parameter 400 is not a rate limit", ErrGeocoderRejected{Status: http.StatusBadRequest, Code: "-2"}, false},
		{"400 with no code is not a rate limit", ErrGeocoderRejected{Status: http.StatusBadRequest}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.RateLimited(); got != tt.want {
				t.Errorf("RateLimited() = %v, want %v", got, tt.want)
			}
		})
	}
}
