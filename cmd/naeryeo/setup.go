package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
)

// runSetup reads an API key from stdin and stores it via save. Without flags
// it targets the ODsay route-search key; with --geocoder it targets the
// place-search (Kakao) key. It is split out from main so the prompt/save
// flow can be exercised by tests with fake stdin and a fake save function.
func runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer, save func(config.Credential, string) error) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	geocoder := fs.Bool("geocoder", false, "장소 검색(Kakao) API 키를 등록한다")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cred := config.ODsayAPIKey
	prompt := "ODsay API Key: "
	if *geocoder {
		cred = config.GeocoderAPIKey
		prompt = "Kakao REST API Key: "
	}

	if _, err := fmt.Fprint(stdout, prompt); err != nil {
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

	if err := save(cred, apiKey); err != nil {
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
