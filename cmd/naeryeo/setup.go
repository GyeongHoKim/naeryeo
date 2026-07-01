package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

// runSetup reads an ODsay API key from stdin and stores it via save. It is
// split out from main so the prompt/save flow can be exercised by tests
// with fake stdin and a fake save function.
func runSetup(_ []string, stdin io.Reader, stdout, stderr io.Writer, save func(string) error) int {
	if _, err := fmt.Fprint(stdout, "ODsay API Key: "); err != nil {
		return 1
	}

	scanner := bufio.NewScanner(stdin)
	var apiKey string
	if scanner.Scan() {
		apiKey = strings.TrimSpace(scanner.Text())
	}

	if apiKey == "" {
		if _, err := fmt.Fprintln(stderr, "naeryeo setup: API 키를 입력해야 합니다"); err != nil {
			return 1
		}
		return 1
	}

	if err := save(apiKey); err != nil {
		if errors.Is(err, config.ErrKeychainUnavailable) {
			if _, werr := fmt.Fprintf(stderr, "naeryeo setup: OS 키체인을 사용할 수 없습니다: %v\n", err); werr != nil {
				return 1
			}
			return 1
		}
		if _, werr := fmt.Fprintf(stderr, "naeryeo setup: 저장 실패: %v\n", err); werr != nil {
			return 1
		}
		return 1
	}

	if _, err := fmt.Fprintln(stdout, "OS 키체인에 저장 완료"); err != nil {
		return 1
	}
	return 0
}
