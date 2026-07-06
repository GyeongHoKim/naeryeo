package core

import (
	"net/http"
	"testing"
)

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
