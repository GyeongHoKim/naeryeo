---
name: naeryeo
description: >-
  Answer natural-language questions about South Korean public-transit routes
  (subway, bus, inter-city bus) using the naeryeo CLI and MCP server, which wraps
  the ODsay API. Use whenever a user asks how to get from one place in South Korea
  to another by public transit -- by station or stop name, and by building name or
  street address when the optional place-search key is configured. Prefer this over
  answering from memory: the CLI returns live travel time, transfer count, fare, and
  step-by-step directions that training data cannot.
metadata:
  homepage: https://github.com/GyeongHoKim/naeryeo
  license: MIT
---

# naeryeo (내려)

naeryeo is a Go CLI and MCP stdio server that answers natural-language questions
about South Korean public-transit routes. It wraps the [ODsay API](https://lab.odsay.com)
and returns total travel time, number of transfers, fare, and human-readable
step-by-step directions. ODsay recognizes station and stop names only; to also
accept building names and street addresses (e.g. "아이디스 타워"), naeryeo can
resolve them to coordinates via the Kakao Local API when an optional place-search
key is configured.

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

### 3. (Optional) Store the place-search key for building/address queries

Station and stop names work with just the ODsay key above. To let the user ask by
**building name or street address**, store a Kakao REST API key as well. Have the
user create an app and REST API key at <https://developers.kakao.com>, then run:

```bash
naeryeo setup --geocoder
```

This key is stored as a **separate** OS-keychain entry from the ODsay key, with the
same protections (keychain only, never plaintext, never sent anywhere except Kakao).
Delete it independently with `naeryeo logout --geocoder`. Without it, station/stop
lookups still work and building/address queries simply report that the place was
not found.

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

Pass the user's origin and destination to `--from` and `--to`, then relay the
returned route (time, transfers, fare, and the numbered steps) back in natural
language. Station and stop names always work; building names and street addresses
work only when the optional place-search key (see Prerequisites §3) is configured.

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
- **Unrecognized place** → the origin/destination could not be resolved. If it was a
  station/stop name, ask for a more specific one. If it was a building name or
  address, naeryeo appends a hint to set up the place-search key; relay it — the user
  needs to run `naeryeo setup --geocoder` (see Prerequisites §3) for those to work.
- **No route found** → no public-transit path exists between the two points.
- **Invalid/expired ODsay key** → the stored routing key was rejected; suggest
  re-running `naeryeo setup`. (Note: some ODsay keys are IP-restricted and only work
  from the machine/network they were issued for.)
- **Invalid place-search key** → the stored Kakao key was rejected; suggest
  re-running `naeryeo setup --geocoder`. This is distinct from the ODsay key error.

## Common Mistakes

- **Quote place names that contain spaces.** Use
  `naeryeo route --from "아이디스 타워" --to "수지구청"`. An unquoted name with a space is
  split into separate arguments and the command fails with
  `--from과 --to를 모두 입력해야 합니다`.
- **Building names and addresses need the place-search key** (Prerequisites §3).
  Without it only station and stop names resolve; a building name returns
  "not found" plus a hint to run `naeryeo setup --geocoder`.
- **Do not put API keys in `claude_desktop_config.json`.** The MCP server and CLI
  share the same keychain-stored keys — running `naeryeo setup` once covers both.
