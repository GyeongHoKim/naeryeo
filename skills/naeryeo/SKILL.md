---
name: naeryeo
description: Answer natural-language questions about South Korean public-transit routes (bus, subway, inter-city) using the naeryeo CLI and MCP server, which wraps the ODsay API. Use when a user asks how to get from one Korean place/station to another by public transit.
metadata:
  homepage: https://github.com/GyeongHoKim/naeryeo
  license: MIT
---

# naeryeo (내려)

naeryeo is a Go CLI and MCP stdio server that answers natural-language questions
about South Korean public-transit routes. It wraps the [ODsay API](https://lab.odsay.com)
and returns total travel time, number of transfers, fare, and human-readable
step-by-step directions.

## When to use this skill

Use it whenever the user asks how to travel between two places in South Korea by
public transit — subway, bus, or inter-city bus. Example questions:

- "지금 강남역에서 홍대입구역까지 대중교통으로 어떻게 가?"
- "How do I get from Seoul Station to Incheon Airport by transit?"

Do **not** use it for driving/walking directions or for transit outside South
Korea — ODsay only covers Korean public transit.

## Prerequisites

### 1. Install `naeryeo`

```bash
# macOS / Linux (Homebrew)
brew install --cask GyeongHoKim/tap/naeryeo

# Windows (Scoop)
scoop bucket add naeryeo https://github.com/GyeongHoKim/scoop-bucket
scoop install naeryeo
```

Verify with `naeryeo --version`.

### 2. Store the ODsay API key (one-time)

Routing requires an ODsay API key. If none is stored, `naeryeo` commands fail with
a message telling the user to run setup first. Have the user obtain a key at
<https://lab.odsay.com>, then run:

```bash
naeryeo setup
```

The key is stored only in the OS keychain (macOS Keychain / Windows Credential
Manager / Linux Secret Service) — never in a plaintext file and never sent
anywhere except ODsay. On a headless Linux host without Secret Service, `setup`
refuses to run rather than falling back to a plaintext file.

Run `naeryeo logout` to delete the stored key.

## Usage

### Option A — CLI (one-off query)

```bash
naeryeo route --from "강남역" --to "홍대입구역"
```

Example output:

```
강남역 → 홍대입구역 (약 42분, 환승 1회)

1. 강남역에서 2호선 승차 (구로디지털단지 방면)
2. 신도림역에서 2호선 → 경의중앙선 환승
3. 홍대입구역 하차

요금: 1,500원
```

Pass the user's origin and destination — station names, stop names, or addresses —
to `--from` and `--to`, then relay the returned route (time, transfers, fare, and
the numbered steps) back in natural language.

### Option B — MCP server (persistent, for chat clients)

For Claude Desktop / Claude Code, register the stdio MCP server once so transit
questions can be answered inline without shelling out each time. Add to
`claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "naeryeo": {
      "command": "naeryeo",
      "args": ["mcp"]
    }
  }
}
```

No API key goes in this config — the server reuses the key stored by `naeryeo setup`.

## Handling errors

Relay the underlying reason to the user; do not retry blindly.

- **No API key stored** → tell the user to run `naeryeo setup` first.
- **Unrecognized place** → ODsay could not resolve the origin/destination; ask the
  user for a more specific station/stop name or address.
- **No route found** → no public-transit path exists between the two points.
- **Invalid/expired key** → the stored key was rejected; suggest re-running
  `naeryeo setup`. (Note: some ODsay keys are IP-restricted and only work from the
  machine/network they were issued for.)
