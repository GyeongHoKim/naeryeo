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

Always pass `--json`. It returns one machine-readable document on stdout for
both success and failure, so you never have to parse Korean prose:

```bash
naeryeo route --from "강남역" --to "홍대입구역" --json
```

Success:

```json
{
  "totalTimeMinutes": 42,
  "transferCount": 1,
  "fareWon": 1500,
  "steps": [
    "강남역에서 2호선 승차 (구로디지털단지 방면)",
    "신도림역에서 2호선 → 경의중앙선 환승",
    "홍대입구역 하차"
  ]
}
```

**Check the `error` key to tell success from failure** — that single key is the
signal. It is absent on success and present on failure. The exit code says the
same thing (0 / 1), so either works.

Two success fields deserve care:

- `noTravelNeeded: true` means the two points are effectively the same place, so
  no trip is needed. It is not a failure.
- `steps` is already ordered; relay it in order.

Pass the user's origin and destination to `--from` and `--to`, then relay the
route (time, transfers, fare, steps) back in natural language. Station and stop
names always work; building names and street addresses work only when the
optional place-search key (see Prerequisites §3) is configured.

Without `--json` the same command prints a human-readable summary instead —
useful if you are showing raw command output to the user, but prefer `--json`
for anything you have to interpret.

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

A failure looks like this — `code` is what you branch on, `message` is what you
relay to the user:

```json
{
  "error": {
    "code": "geocoder_rate_limited",
    "message": "장소 검색 요청이 일시적으로 제한되었습니다. 잠시 후 다시 시도하세요"
  }
}
```

**Branch on `code`, never on `message`.** Message wording can change at any
time; codes are stable. Relay `message` to the user as-is, and relay `hint` too
when it is present.

| `code` | What to do | Retry? |
| --- | --- | :---: |
| `api_key_missing` | Tell the user to run `naeryeo setup` | ❌ |
| `auth_failed` | The routing key was rejected — tell the user to re-run `naeryeo setup`. Some ODsay keys are IP-restricted and only work from the machine they were issued for | ❌ |
| `geocoder_auth_failed` | The place-search key was rejected — tell the user to re-run `naeryeo setup --geocoder` | ❌ |
| `geocoder_forbidden` | **Different from the above**: the key is valid but the Kakao app's settings deny it. Tell the user to enable the 카카오맵(로컬) service and check domain/IP restrictions in the Kakao console. Re-registering the key will NOT help | ❌ |
| `geocoder_rate_limited` | A transient rate limit. **Send the same request again shortly** | ✅ |
| `geocoder_rejected` | **Different from the above**: the location could not be resolved. Reformulate — use a road-name/lot-number address or a nearby station name. Resending the same input is pointless | ❌ |
| `point_not_found` | Read `side` (`from` / `to` / `both`) and re-ask about only that endpoint. `name` is the input that failed | ❌ |
| `no_route` | No transit route connects the two points. Report it | ❌ |
| `geocoder_unavailable` | Place-search service unreachable | ✅ |
| `upstream_unavailable` | Routing service unreachable | ✅ |
| `upstream_rejected` | The routing service refused the request. Report it and suggest checking the endpoints | ❌ |
| `credential_store_error` | The OS keychain could not be read. Relay the `hint` | ❌ |
| `invalid_arguments` | The command was malformed. Fix the invocation and retry | ❌ |
| `internal_error` | Unexpected. Report it to the user | ❌ |

If you see a code that is not in this table, relay `message` and do not retry.

The two pairs worth re-reading: `geocoder_rate_limited` vs `geocoder_rejected`
(retry vs reformulate), and `geocoder_auth_failed` vs `geocoder_forbidden`
(re-register vs fix console settings). Getting these backwards means either a
pointless retry loop or telling the user to redo something that cannot help.

## Common Mistakes

- **Quote place names that contain spaces.** Use
  `naeryeo route --from "아이디스 타워" --to "수지구청" --json`. An unquoted name with a
  space is split into separate arguments and the command fails with
  `invalid_arguments`.
- **Do not match on error text.** Branch on `error.code`. The Korean wording in
  `message` is for the user, not for you, and it is free to change.
- **`noTravelNeeded` is a success, not a failure.** It means the two points are
  close enough that no trip is needed — do not report it as "no route found"
  (that is `no_route`).
- **Building names and addresses need the place-search key** (Prerequisites §3).
  Without it only station and stop names resolve; a building name returns
  `point_not_found` plus a `hint` to run `naeryeo setup --geocoder`.
- **`--debug` writes to stderr, never to stdout.** Combining it with `--json`
  is safe: stdout stays a single parseable document.
- **Do not put API keys in `claude_desktop_config.json`.** The MCP server and CLI
  share the same keychain-stored keys — running `naeryeo setup` once covers both.
