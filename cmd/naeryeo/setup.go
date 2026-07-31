package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GyeongHoKim/naeryeo/internal/config"
	"github.com/GyeongHoKim/naeryeo/internal/core"
	"github.com/GyeongHoKim/naeryeo/internal/motis"
)

// defaultMotisURL is what a MOTIS instance started from the recipe in
// deploy/motis listens on. It is offered as the pre-filled answer so the
// common case is a single Enter.
const defaultMotisURL = "http://localhost:8080"

// probeTimeout bounds the reachability check. It is short because the user is
// sitting at a prompt: a self-hosted engine on the local network answers in
// milliseconds, and anything slower is a problem worth reporting now rather
// than at every later search.
const probeTimeout = 5 * time.Second

// setupDeps is everything runSetup touches outside its own process. Passing
// them in keeps the whole wizard — including the reachability probe —
// exercisable with fakes and a scripted stdin, with no pty and no network.
type setupDeps struct {
	SaveCredential   func(config.Credential, string) error
	LoadCredential   func(config.Credential) (string, error)
	DeleteCredential func(config.Credential) error
	SaveSettings     func(config.Settings) error
	ProbeMotis       func(ctx context.Context, baseURL string) error
}

// runSetup configures naeryeo: which routing provider to use, that provider's
// credential or address, and whether place search is enabled. It replaces both
// the single-key setup and the separate logout command — the state it writes
// has grown from one API key to a provider, an address, a geocoder choice and
// two credentials, and splitting that across two commands would make the user
// track which command owns which piece (spec 006 FR-035).
//
// Interactive and non-interactive are the same code path: a flag supplies an
// answer, and any question left unanswered is prompted for. That keeps the
// line-oriented shape the tests drive with a fake stdin, and means a TUI
// framework is not needed — nor wanted, since it would replace a testable
// reader with a terminal emulator.
func runSetup(args []string, stdin io.Reader, stdout, stderr io.Writer, deps setupDeps) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "경로 검색 제공자: motis(자체 호스팅) 또는 odsay")
	motisURL := fs.String("motis-url", "", "자체 호스팅한 MOTIS 서버 주소 (예: "+defaultMotisURL+")")
	geocoder := fs.String("geocoder", "", "장소 검색: kakao 또는 none")
	del := fs.String("delete", "", "저장된 자격증명 삭제: odsay, kakao, all")

	if err := fs.Parse(args); err != nil {
		reportGeocoderFlagMigration(args, stderr)
		return 1
	}

	// Deleting is a mode of its own: mixing it with configuration in one
	// invocation would make "did that also change my provider?" a question
	// the user has to ask.
	if *del != "" {
		if *provider != "" || *motisURL != "" || *geocoder != "" {
			return setupError(stderr, "--delete는 다른 옵션과 함께 사용할 수 없습니다")
		}
		return runDelete(*del, stdout, stderr, deps)
	}

	in := bufio.NewScanner(stdin)

	choice, code := resolveProvider(*provider, in, stdout, stderr)
	if code != 0 {
		return code
	}
	if choice == choiceDelete {
		target, dcode := promptDeleteTarget(in, stdout, stderr)
		if dcode != 0 {
			return dcode
		}
		return runDelete(target, stdout, stderr, deps)
	}

	settings := config.Settings{RoutingProvider: config.RoutingProvider(choice)}

	switch settings.RoutingProvider {
	case config.ProviderMotis:
		url, ucode := resolveMotisURL(*motisURL, in, stdout, stderr)
		if ucode != 0 {
			return ucode
		}
		settings.MotisURL = url
		if pcode := probeEngine(url, stdout, stderr, deps); pcode != 0 {
			return pcode
		}
	case config.ProviderODsay:
		if _, err := fmt.Fprintln(stdout, "\nODsay(lab.odsay.com)에서 발급받은 앱키가 필요합니다."); err != nil {
			return 1
		}
		if kcode := promptAndStoreKey(config.ODsayAPIKey, "ODsay API Key: ", in, stdout, stderr, deps); kcode != 0 {
			return kcode
		}
	}

	geoChoice, gcode := resolveGeocoder(*geocoder, in, stdout, stderr)
	if gcode != 0 {
		return gcode
	}
	settings.Geocoder = geoChoice
	if geoChoice == config.GeocoderKakao {
		if kcode := promptAndStoreKey(config.GeocoderAPIKey, "Kakao REST API Key: ", in, stdout, stderr, deps); kcode != 0 {
			return kcode
		}
	}

	if ccode := confirmAndSave(settings, in, stdout, stderr, deps); ccode != 0 {
		return ccode
	}
	return 0
}

// choiceDelete is the wizard's third top-level option. It is not a provider,
// so it is kept out of config.RoutingProvider's value space.
const choiceDelete = "delete"

// resolveProvider answers "which engine" from the flag if given, else by
// asking. The self-hosted option is first and is the Enter default: it is the
// path that does not depend on anyone's pricing policy, so it is what a user
// with no preference should land on (spec 006 FR-037).
func resolveProvider(flagValue string, in *bufio.Scanner, stdout, stderr io.Writer) (string, int) {
	if flagValue != "" {
		switch config.RoutingProvider(flagValue) {
		case config.ProviderMotis, config.ProviderODsay:
			return flagValue, 0
		default:
			return "", setupError(stderr, fmt.Sprintf("--provider는 %q 또는 %q여야 합니다", config.ProviderMotis, config.ProviderODsay))
		}
	}

	menu := "" +
		"경로 검색 제공자를 선택하세요.\n" +
		"  1) 자체 호스팅 (MOTIS) — API 키가 필요 없습니다  [기본]\n" +
		"  2) ODsay — 앱키 발급이 필요합니다\n" +
		"  3) 저장된 자격증명 삭제\n"
	if _, err := fmt.Fprint(stdout, menu); err != nil {
		return "", 1
	}

	answer, ok := prompt(in, stdout, "선택 [1]: ")
	if !ok {
		return "", setupError(stderr, "입력을 읽을 수 없습니다")
	}
	switch answer {
	case "", "1":
		return string(config.ProviderMotis), 0
	case "2":
		return string(config.ProviderODsay), 0
	case "3":
		return choiceDelete, 0
	default:
		return "", setupError(stderr, "1, 2, 3 중에서 선택하세요")
	}
}

func resolveMotisURL(flagValue string, in *bufio.Scanner, stdout, stderr io.Writer) (string, int) {
	if flagValue != "" {
		return flagValue, 0
	}

	intro := "\nMOTIS 서버 주소를 입력하세요. API 키는 필요하지 않습니다.\n"
	if _, err := fmt.Fprint(stdout, intro); err != nil {
		return "", 1
	}
	answer, ok := prompt(in, stdout, fmt.Sprintf("주소 [%s]: ", defaultMotisURL))
	if !ok {
		return "", setupError(stderr, "입력을 읽을 수 없습니다")
	}
	if answer == "" {
		return defaultMotisURL, 0
	}
	return answer, 0
}

// probeEngine refuses an endpoint that is not going to work, before it is
// written anywhere.
//
// Doing this at configuration time is what makes "the engine is up but has no
// timetable loaded" distinguishable from "there is no route between these two
// points" — at search time those two look identical, since both produce an
// empty result (spec 006 FR-016). Paying one round trip here beats paying one
// on every later search.
func probeEngine(baseURL string, stdout, stderr io.Writer, deps setupDeps) int {
	if deps.ProbeMotis == nil {
		return 0
	}
	if _, err := fmt.Fprint(stdout, "\n  연결 확인 중... "); err != nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	err := deps.ProbeMotis(ctx, baseURL)
	if err == nil {
		if _, werr := fmt.Fprintln(stdout, "정상"); werr != nil {
			return 1
		}
		return 0
	}
	if _, werr := fmt.Fprintln(stdout, "실패"); werr != nil {
		return 1
	}

	// The address the user just typed is not echoed back into these messages:
	// it is their private network, and setup's output can end up in a
	// terminal recording or a support paste.
	var rejected *core.ErrMotisRejected
	switch {
	case errors.Is(err, motis.ErrNoData):
		return setupErrorWithDocs(stderr, "엔진은 응답하지만 시간표·지도 데이터가 적재되지 않은 것으로 보입니다")
	case errors.As(err, &rejected):
		return setupErrorWithDocs(stderr, "엔진이 요청을 거부했습니다. 주소가 MOTIS 서버가 맞는지 확인하세요")
	default:
		return setupErrorWithDocs(stderr, "엔진에 연결할 수 없습니다. 주소와 실행 상태를 확인하세요")
	}
}

func resolveGeocoder(flagValue string, in *bufio.Scanner, stdout, stderr io.Writer) (config.GeocoderChoice, int) {
	if flagValue != "" {
		switch config.GeocoderChoice(flagValue) {
		case config.GeocoderKakao, config.GeocoderNone:
			return config.GeocoderChoice(flagValue), 0
		default:
			return "", setupError(stderr, fmt.Sprintf("--geocoder는 %q 또는 %q여야 합니다", config.GeocoderKakao, config.GeocoderNone))
		}
	}

	menu := "" +
		"\n건물명·주소로도 검색하시겠습니까? (역·정류장 이름만 쓸 경우 필요 없습니다)\n" +
		"  1) 사용 안 함  [기본]\n" +
		"  2) Kakao 장소 검색 사용 — REST API 키가 필요합니다\n"
	if _, err := fmt.Fprint(stdout, menu); err != nil {
		return "", 1
	}

	answer, ok := prompt(in, stdout, "선택 [1]: ")
	if !ok {
		return "", setupError(stderr, "입력을 읽을 수 없습니다")
	}
	switch answer {
	case "", "1":
		return config.GeocoderNone, 0
	case "2":
		return config.GeocoderKakao, 0
	default:
		return "", setupError(stderr, "1 또는 2 중에서 선택하세요")
	}
}

// promptAndStoreKey reads a secret from stdin and stores it in the OS
// keychain. Secrets are never accepted as flags: a command line lands in
// shell history and in every `ps` listing on the machine, which is the same
// exposure GYE-293 removed from error output (spec 006 FR-006).
func promptAndStoreKey(cred config.Credential, label string, in *bufio.Scanner, stdout, stderr io.Writer, deps setupDeps) int {
	key, ok := prompt(in, stdout, label)
	if !ok || key == "" {
		return setupError(stderr, "API 키를 입력해야 합니다")
	}

	if err := deps.SaveCredential(cred, key); err != nil {
		if errors.Is(err, config.ErrKeychainUnavailable) {
			return setupError(stderr, "OS 키체인을 사용할 수 없습니다. 접근 권한을 확인하세요")
		}
		return setupError(stderr, "자격증명을 저장하지 못했습니다")
	}
	return 0
}

// confirmAndSave shows what is about to be written and asks once. The MOTIS
// address IS shown here — the user typed it moments ago and needs to check
// it — which is the one place it appears in output by design.
func confirmAndSave(s config.Settings, in *bufio.Scanner, stdout, stderr io.Writer, deps setupDeps) int {
	routing := "ODsay"
	if s.RoutingProvider == config.ProviderMotis {
		routing = "자체 호스팅 (MOTIS) — " + s.MotisURL
	}
	place := "사용 안 함"
	if s.Geocoder == config.GeocoderKakao {
		place = "Kakao"
	}

	summary := fmt.Sprintf("\n설정 요약\n  경로 검색: %s\n  장소 검색: %s\n", routing, place)
	if _, err := fmt.Fprint(stdout, summary); err != nil {
		return 1
	}

	answer, ok := prompt(in, stdout, "저장하시겠습니까? [Y/n]: ")
	if !ok {
		return setupError(stderr, "입력을 읽을 수 없습니다")
	}
	if isNo(answer) {
		if _, err := fmt.Fprintln(stdout, "저장하지 않았습니다"); err != nil {
			return 1
		}
		return 1
	}

	if err := deps.SaveSettings(s); err != nil {
		return setupError(stderr, "설정을 저장하지 못했습니다")
	}
	if _, err := fmt.Fprintln(stdout, "저장 완료"); err != nil {
		return 1
	}
	return 0
}

func promptDeleteTarget(in *bufio.Scanner, stdout, stderr io.Writer) (string, int) {
	menu := "" +
		"\n삭제할 자격증명을 선택하세요.\n" +
		"  1) ODsay API 키\n" +
		"  2) Kakao 장소 검색 키\n" +
		"  3) 모두\n"
	if _, err := fmt.Fprint(stdout, menu); err != nil {
		return "", 1
	}
	answer, ok := prompt(in, stdout, "선택: ")
	if !ok {
		return "", setupError(stderr, "입력을 읽을 수 없습니다")
	}
	switch answer {
	case "1":
		return "odsay", 0
	case "2":
		return "kakao", 0
	case "3":
		return "all", 0
	default:
		return "", setupError(stderr, "1, 2, 3 중에서 선택하세요")
	}
}

// runDelete removes stored credentials. It deliberately leaves the settings
// file alone: the user asked to drop a key, not to forget which engine they
// use, and conflating the two would make "clear my API key" silently
// un-configure the tool.
//
// The load-then-delete shape is inherited from the command this replaced:
// config.Delete is idempotent and reports nothing either way, so the only way
// to tell "deleted" from "there was nothing there" is to look first.
func runDelete(target string, stdout, stderr io.Writer, deps setupDeps) int {
	var creds []config.Credential
	switch target {
	case "odsay":
		creds = []config.Credential{config.ODsayAPIKey}
	case "kakao":
		creds = []config.Credential{config.GeocoderAPIKey}
	case "all":
		creds = []config.Credential{config.ODsayAPIKey, config.GeocoderAPIKey}
	default:
		return setupError(stderr, "--delete는 odsay, kakao, all 중 하나여야 합니다")
	}

	deleted := 0
	for _, cred := range creds {
		_, loadErr := deps.LoadCredential(cred)
		had := !errors.Is(loadErr, config.ErrNotConfigured)

		if err := deps.DeleteCredential(cred); err != nil {
			if errors.Is(err, config.ErrKeychainUnavailable) {
				return setupError(stderr, "OS 키체인을 사용할 수 없습니다. 접근 권한을 확인하세요")
			}
			return setupError(stderr, "자격증명을 삭제하지 못했습니다")
		}
		if had {
			deleted++
		}
	}

	message := "저장된 API 키를 삭제했습니다"
	if deleted == 0 {
		message = "삭제할 API 키가 없습니다"
	}
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		return 1
	}
	return 0
}

// prompt writes label and reads one line. It returns ok=false only when the
// input ended, which the caller reports rather than treating as an empty
// answer — silently defaulting on a closed pipe would save a configuration
// the user never confirmed.
func prompt(in *bufio.Scanner, stdout io.Writer, label string) (string, bool) {
	if _, err := fmt.Fprint(stdout, label); err != nil {
		return "", false
	}
	if !in.Scan() {
		return "", false
	}
	return strings.TrimSpace(in.Text()), true
}

func isNo(answer string) bool {
	switch strings.ToLower(answer) {
	case "n", "no", "아니오":
		return true
	default:
		return false
	}
}

func setupError(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintln(stderr, "naeryeo setup: "+message); err != nil {
		return 1
	}
	return 1
}

// setupErrorWithDocs adds the self-hosting guide to a failure the user has to
// fix in their own infrastructure. It is the same link the route command's
// self-hosting error codes carry, so the answer is reachable from wherever
// the user first hits the problem.
func setupErrorWithDocs(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintln(stderr, "naeryeo setup: "+message+"\n"+selfHostingDocsURL); err != nil {
		return 1
	}
	return 1
}

// reportGeocoderFlagMigration explains the one breaking change a user is most
// likely to trip over. --geocoder used to be a boolean, so the old habit of
// writing it bare now consumes the next argument or fails outright; a generic
// parse error would leave the user to guess why a command that worked
// yesterday does not today (spec 006 FR-036).
func reportGeocoderFlagMigration(args []string, stderr io.Writer) {
	for _, a := range args {
		if a == "--geocoder" || a == "-geocoder" {
			_, _ = fmt.Fprintln(stderr,
				"naeryeo setup: --geocoder는 이제 값을 받습니다. --geocoder=kakao 또는 --geocoder=none 형태로 지정하세요")
			return
		}
	}
}
