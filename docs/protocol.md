# Backend protocol v1

One JSON object per line, maximum input line size 64 KiB. The backend serializes
commands and polling. EOF, SIGINT, or SIGTERM exits. The QML service owns one
backend shared by all widget instances.

JSON uses UTF-8 and unique object member names. Command fields are case-sensitive;
invalid UTF-8 or duplicate members produce an `invalid command JSON` result.
Required numeric and boolean fields must be present and non-null. Explicit zero
and false values remain valid where allowed, including volume 0, mute false,
delta 0, and comment offset 0.

Every command has a string `id` and an `op`. Results are
`{"type":"result","id":"1","ok":true}` or include `ok:false` and `error`.
A snapshot follows every command, including errors. Snapshots have `version:1`,
`status` (`starting`, `ready`, `offline`), `rooms`, `selected`, `playback`,
`volume`, `mute`, `account`, and `rp`. Session tokens/passwords are never included.
The `backendVersion` field comes from the embedded plugin manifest; `version`
remains the protocol version. `rpMixes` contains the supported `{id,name}` choices,
including Automatic (-1), and drives the widget's mix selector.
`rp.song` is null until matched against the speaker. Comment failures are separate
from metadata failures so they do not disable rating an identified track.

| Operation | Additional fields | Effect |
| --- | --- | --- |
| `refresh` | — | Read topology, playback, and RP metadata |
| `selectRoom` | `room`: Sonos UID | Persist the selected anchor room |
| `playPause` | — | Play or pause the group coordinator |
| `next`, `previous` | — | Skip if advertised by Sonos |
| `setVolume` | `volume`: 0–100 | Set selected room volume |
| `adjustVolume` | `delta`: integer | Change selected room volume, clamped 0–100 |
| `setMute` | `mute`: boolean | Mute selected room |
| `rpChannel` | `channel`: channel ID or -1 | Persist room mix override; -1 clears |
| `rpLogin` | `username`, `password` | Authenticate and persist session token |
| `rpLogout` | — | Remove session file |
| `rpRate` | `room`, `songId`, `rating`: 1–10 | Revalidate selected room/track and submit |
| `rpComments` | `songId`, `offset` | With RP authentication, load first page (0) or next advertised offset |

Use the popup for account operations: typing a password into a shell command can
leave it in command history. Requests have bounded deadlines and are never
automatically replayed after a backend crash.
