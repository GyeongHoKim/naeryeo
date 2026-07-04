package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/naeryeo/internal/core"
)

// renderRouteMarkdown turns a core.RouteResult into the refined markdown
// text the cloud (PlayMCP) tool returns. Format is fixed by
// specs/005-playmcp-cloud-server/contracts/mcp-tool.md: bold header with
// duration/transfers (+fare only when known), a numbered step list, and a
// data-source footnote. dataSource names whichever backend actually served
// the request (e.g. "TMAP 대중교통 API", "MOTIS(KTDB·OSM)") — the footnote
// must never claim a fixed provider regardless of which one answered. Raw
// backend JSON must never appear here — the PlayMCP dev guide requires
// minimal, human-readable tool results.
func renderRouteMarkdown(from, to, dataSource string, r core.RouteResult) string {
	if r.NoTravelNeeded {
		return fmt.Sprintf("**%s → %s**\n\n출발지와 도착지가 사실상 같은 위치예요. 대중교통 이동이 필요하지 않습니다.", from, to)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%s → %s** · 약 %d분 · 환승 %d회", from, to, r.TotalTime, r.TransferCount)
	if r.Fare > 0 {
		fmt.Fprintf(&b, " · 요금 %s원", formatKRW(r.Fare))
	}
	b.WriteString("\n\n")

	for i, step := range r.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step.Description)
	}

	fmt.Fprintf(&b, "\n_데이터: %s 기반 시간표 — 실시간 지연 미반영_", dataSource)
	return b.String()
}

// formatKRW renders an amount with thousands separators (1500 → "1,500").
func formatKRW(amount int) string {
	s := strconv.Itoa(amount)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
