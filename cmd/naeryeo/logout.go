package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

// runLogout removes the stored ODsay API key via del. It calls load first
// purely to choose the right user-facing message (FR-009: "nothing to
// delete" must read differently from "deleted"), since Delete itself is
// intentionally idempotent and reports no error either way.
func runLogout(_ []string, stdout, stderr io.Writer, load func() (string, error), del func() error) int {
	_, loadErr := load()
	hadKey := !errors.Is(loadErr, config.ErrNotConfigured)

	if err := del(); err != nil {
		if errors.Is(err, config.ErrKeychainUnavailable) {
			if _, werr := fmt.Fprintf(stderr, "naeryeo logout: OS 키체인을 사용할 수 없습니다: %v\n", err); werr != nil {
				return 1
			}
			return 1
		}
		if _, werr := fmt.Fprintf(stderr, "naeryeo logout: 삭제 실패: %v\n", err); werr != nil {
			return 1
		}
		return 1
	}

	message := "저장된 API 키를 삭제했습니다"
	if !hadKey {
		message = "삭제할 API 키가 없습니다"
	}
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		return 1
	}
	return 0
}
