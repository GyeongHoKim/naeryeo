package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// runRoute parses --from/--to, resolves the stored API key via load, and
// searches for a route via findRoute. It is split out from main so it can
// be exercised by tests with fake load/findRoute functions.
func runRoute(
	args []string,
	stdout, stderr io.Writer,
	load func() (string, error),
	loadGeocoder func() (string, error),
	findRoute func(ctx context.Context, apiKey, from, to string) (core.RouteResult, error),
) int {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "출발지 (역/정류장 이름 또는 주소)")
	to := fs.String("to", "", "도착지 (역/정류장 이름 또는 주소)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *from == "" || *to == "" {
		if _, err := fmt.Fprintln(stderr, "naeryeo route: --from과 --to를 모두 입력해야 합니다"); err != nil {
			return 1
		}
		return 1
	}

	apiKey, loadErr := load()
	if errors.Is(loadErr, config.ErrNotConfigured) {
		if _, err := fmt.Fprintln(stderr, "naeryeo route: API 키가 설정되지 않았습니다. naeryeo setup을 먼저 실행하세요"); err != nil {
			return 1
		}
		return 1
	}
	if loadErr != nil {
		if _, err := fmt.Fprintf(stderr, "naeryeo route: API 키 조회 실패: %v\n", loadErr); err != nil {
			return 1
		}
		return 1
	}

	result, err := findRoute(context.Background(), apiKey, *from, *to)
	if err != nil {
		code, writeErr := reportRouteError(stderr, err, geocoderConfigured(loadGeocoder))
		if writeErr != nil {
			return 1
		}
		return code
	}

	return printRouteResult(stdout, *from, *to, result)
}

// geocoderConfigured reports whether a place-search key is stored. runRoute
// and the MCP tool handler use it to decide whether to append the FR-007
// "set up --geocoder" hint: the hint only helps when no geocoder key exists.
// loadGeocoder is only called on an error path, so the happy path does not
// touch the keychain a second time.
func geocoderConfigured(loadGeocoder func() (string, error)) bool {
	key, err := loadGeocoder()
	return err == nil && key != ""
}

// reportRouteError writes a message describing err to stderr. It always
// returns 1, since this is only called on an already-known error path; a
// write failure while reporting that error is still surfaced via the
// returned error so the caller cannot silently ignore it.
func reportRouteError(stderr io.Writer, err error, geocoderConfigured bool) (int, error) {
	_, writeErr := fmt.Fprintln(stderr, "naeryeo route: "+routeErrorMessage(err, geocoderConfigured))
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
func routeErrorMessage(err error, geocoderConfigured bool) string {
	var pointErr *core.ErrPointNotFound
	switch {
	case errors.Is(err, core.ErrAPIKeyMissing):
		return "API 키가 설정되지 않았습니다. naeryeo setup을 먼저 실행하세요"
	case errors.Is(err, core.ErrAuthFailed):
		return "저장된 API 키가 유효하지 않습니다. naeryeo setup으로 다시 등록하세요"
	case errors.Is(err, core.ErrGeocoderAuthFailed):
		return "장소 검색 키가 유효하지 않습니다. naeryeo setup --geocoder로 다시 등록하세요"
	case errors.Is(err, core.ErrGeocoderForbidden):
		return "장소 검색 서비스(Kakao) 권한이 거부되었습니다. Kakao 개발자 콘솔에서 해당 앱의 카카오맵(로컬) 서비스를 활성화했는지, 키의 도메인·IP 제한 설정을 확인하세요"
	case errors.As(err, &pointErr):
		msg := fmt.Sprintf("%s을(를) 찾을 수 없습니다: %q", sideLabel(pointErr.Side), pointErr.Name)
		if !geocoderConfigured {
			msg += "\n건물명·주소로 찾으려면 naeryeo setup --geocoder로 장소 검색 키를 설정하세요"
		}
		return msg
	case errors.Is(err, core.ErrNoRoute):
		return "두 지점 사이에 대중교통 경로가 없습니다"
	case errors.Is(err, core.ErrGeocoderUnavailable):
		return "장소 검색 서비스에 연결할 수 없습니다. 잠시 후 다시 시도하세요"
	default:
		return fmt.Sprintf("경로 검색 중 오류가 발생했습니다: %v", err)
	}
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
	if _, err := fmt.Fprintf(stdout, "\n요금: %s원\n", withThousandsSeparator(result.Fare)); err != nil {
		return 1
	}
	return 0
}
