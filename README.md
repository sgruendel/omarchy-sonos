# Sonos Paradise

[![build](https://github.com/sgruendel/omarchy-sonos/actions/workflows/build.yaml/badge.svg)](https://github.com/sgruendel/omarchy-sonos/actions/workflows/build.yaml)

An Omarchy Quattro bar plugin with a dependency-free Go backend. Control local
Sonos speakers and see Radio Paradise artwork, listener scores, your rating,
and song comments. Sign in with your RP account to submit ratings from 1 to 10.

Inspired by [OmaSonos](https://github.com/ctl0v0/omasonos) and the RP API
integration in [rptui](https://github.com/pdfrg/rptui). Audio stays on Sonos;
this plugin does not run a second RP player.

## Build and install

Requires Go 1.27+, Bash, an Omarchy Quattro shell with its plugin API, and Sonos
speakers reachable on your LAN. There are no Go module dependencies, Python
packages, Sonos cloud credentials, or runtime downloads.

The backend uses Go's `encoding/json/v2` and `encoding/json/jsontext` packages,
enabled by default in Go 1.27; no `GOEXPERIMENT` setting is needed.

```sh
git clone https://github.com/sgruendel/omarchy-sonos.git
cd omarchy-sonos
make build
make check
./scripts/install-local.sh
```

The installer builds and validates a staged plugin, backs up an existing local
installation, copies it to `~/.config/omarchy/plugins/sgruendel.sonos`, and
enables the widget. `XDG_CONFIG_HOME` is honored. Installation is a separate
step: building and testing the checkout do not change your desktop.

For a distributable plugin folder, run `make stage`. Its Go binary is built for
the current OS/architecture; rebuild for other machines. The source checkout's
`sonos-backend` launcher expects `bin/omarchy-sonos`, so build before enabling it.

GitHub Actions checks formatting, runs race-enabled tests and `go vet`, and builds
the Go backend on pushes to `main` and pull requests. Tags starting with `v` also
publish Linux amd64 and arm64 plugin bundles with SHA-256 checksums to GitHub
Releases. Manual workflow runs build downloadable development artifacts.
Dependabot checks Go modules and GitHub Actions weekly.

The Go entry point and state logic live in `main.go` and `app.go` at the repository
root; `commands.go`, `protocol.go`, and `discovery.go` own command dispatch,
JSON Lines framing, and concurrent discovery probes. The `sonos` package owns
local speaker discovery and UPnP,
`rp` owns Radio Paradise metadata and accounts, and `storage` handles atomic
private JSON state files. Build the backend from the root with `make build`
or `go build .`.

The plugin version is embedded from `manifest.json` into the backend and used
in RP's User-Agent and snapshots. RP mixes also come from the backend, keeping
the widget's selector in sync with supported channels.

## Use

- Left click opens the controller; Escape or an outside click closes it.
- Middle click toggles playback. The wheel changes the selected room's volume
  in steps of 5. Playback commands target that room's group coordinator.
- Select a room to control its session. Volume and mute affect that room only.
- Transport buttons follow the actions Sonos advertises for the current source.
- An RP stream shows matched song artwork, album, and listener rating/count.
  Signing in adds your personal rating and comments, loaded 20 at a time.
- Open **RP account**, sign in, then select a rating from 1 to 10. Sign out removes
  the locally saved session. RP requires account authentication for its comments
  endpoint; the popup prompts you to sign in when needed.

Detection recognizes `radioparadise.com` and `radioparadise.stream` audio hosts
using both Sonos's track and overall playback URLs, plus RP station labels,
including Main, Mellow, RockIt!, The Globe, Beyond, Serenity, and KFAT. An opaque
TuneIn/Sonos URI may not identify the mix. Select an **RP mix override** for that
room; **Automatic** clears it. An override explicitly treats that room's source
as the chosen RP mix until cleared.

The recent RP playlist is matched against Sonos artist/title or stream metadata.
This handles buffered streams without assuming the first song in RP's playlist
is what you hear. If metadata is missing, ambiguous, or outside the recent
playlist, the plugin waits for a match. Ratings are enabled only while playing,
signed in, and matched. Submission refreshes topology and metadata, verifies
the requested song and room, then checks Sonos again before sending the rating.
There is still an unavoidable small window between the last speaker read and
the RP request; these independent systems provide no atomic operation.

## Discovery and troubleshooting

SSDP discovery runs on active IPv4 interfaces. Cached speaker IPs are tried
first. Playback and volume are polled every 3 seconds, topology/discovery every
60 seconds while online and every 10 seconds while offline, the RP playlist
every 15 seconds, and comments every 2 minutes. Up to four discovery probes
run concurrently; only a complete topology cancels the remaining probes. If
cached hosts supply only a partial topology, SSDP locations are also probed.
If every probe is incomplete, discovery keeps the result with the most reachable
rooms across both sets, preferring cached hosts and earlier locations on ties.
Requests have timeouts. No inbound callback listener or subnet scan is used.

If multicast is unavailable, provide a speaker IP when launching the shell with
`SONOS_HOSTS=192.168.1.42` in its environment, or seed the cache using:

```sh
./sonos-backend --hosts 192.168.1.42 --once
```

This reads speakers and writes local discovery state, without changing playback.
It prints a starting snapshot and a final snapshot. Seed addresses are persisted
once a topology is discovered. Multiple IPs can be comma-separated. Allow LAN
UDP SSDP replies and TCP port 1400 to speakers, plus HTTPS to RP and its image CDN.

Use **Refresh** to immediately rediscover rooms and refresh RP metadata. The
service restarts the backend with a capped delay if it exits. Commands run in
order and report errors in the popup; rate requests are not automatically retried.
Backend diagnostics stay visible across commands until replaced by a newer
diagnostic or cleared when the backend exits.

## State and privacy

State lives in `${XDG_STATE_HOME:-~/.local/state}/sgruendel.sonos`:

- `state.json`: cached IPs, selected room, and RP mix overrides.
- `rp-session.json`: RP username, user ID, and password-derived session token.

Files are atomically replaced with mode `0600`; newly created directories use
`0700`. RP session files with group or other permissions are rejected. If the
session cannot be loaded, the backend continues signed out and emits a generic
diagnostic; signing in again replaces the session with a private file. Non-secret
`state.json` remains readable after a restore or migration with looser permissions;
the next successful save resets its permissions to `0600`.
The RP password is sent through the backend's stdin and is not saved or
logged. The session token is a secret stored in a local file, not a keyring. The
widget clears the password field on submission/close. Tokens stay out of stdout.
RP's auth API uses HTTPS query parameters, as in rptui; errors never echo these
URLs, and API redirects are refused. Sign out to delete the saved token.

Only RP metadata/account requests and cover images leave the LAN. Comments are
converted to plain text and rendered with `Text.PlainText`. The backend receives
JSON Lines on stdin and emits `snapshot` and correlated `result` records on
stdout; diagnostics use stderr. See [docs/protocol.md](docs/protocol.md).

## Validation and current scope

`make check` runs race-enabled Go tests, `go vet`, and Omarchy's manifest
validator. Tests use simulated Sonos SOAP and RP HTTP servers, including song
transitions, API failures, auth persistence, and comment pagination. A local
hardware/account acceptance checklist is in [docs/acceptance.md](docs/acceptance.md).

Optional checks: `make check-qml` checks QML with installed Omarchy
imports (Qt may report static type warnings for dynamic shell properties).
`RP_LIVE_TEST=1 go test -run TestLivePublicRP -v ./rp` checks the public RP
playlist endpoint without credentials or mutations.

This first version implements room selection, transport, room volume/mute, RP
metadata, comments, and account ratings. OmaSonos features such as Favorites,
seek, group editing, handoff, and event subscriptions are not implemented.
Stereo satellites are excluded from room selection. S1 compatibility and
real-device behavior need verification. RP's APIs are undocumented and may change.

```sh
omarchy plugin disable sgruendel.sonos
omarchy plugin remove sgruendel.sonos --yes
```

Removing the plugin leaves its private state directory. Sign out first if you
want to remove the RP session; delete the state directory separately to reset it.

MIT licensed. Third-party notices are in [LICENSE](LICENSE). This project is
not affiliated with Sonos, Radio Paradise, or Omarchy.
