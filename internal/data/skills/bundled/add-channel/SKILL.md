---
name: add-channel
description: Connect yourself to a new messaging platform or interface (Discord, Slack, Matrix, WhatsApp, email, a web chat, SMS...) by writing a channel skill; also the reference for the channel protocol used by every channel skill, including telegram. Read before creating, changing or debugging any channel.
---

# Channel skills

A channel is how your owner talks to you. Besides the built-in ones (terminal,
Telegram) any skill directory that contains a `channel.json` is a channel: the
core starts its program (the *bridge*) as a child process and talks to it over
stdin/stdout. You can add a new interface for yourself by writing one — no core
changes, no container restart.

The bridge only moves messages. The core does everything else: it renders
status updates, answers, buttons and long-message splitting, runs the slash
commands, and **checks that every message comes from the owner**.

## Layout

```
/data/skills/<name>/
  SKILL.md       what the channel is, how to set it up, quirks you learned
  channel.json   manifest (below)
  bridge.py      the bridge program (any language; python3 stdlib is safest)
```

`<name>` is the channel name (lowercase, hyphens).

## channel.json

```json
{
  "command": ["python3", "bridge.py"],
  "requires_env": ["DISCORD_BOT_TOKEN"],
  "owner_env": "DISCORD_OWNER_ID",
  "format": "markdown",
  "max_message_len": 2000,
  "can_edit": true,
  "buttons": false,
  "enabled": true
}
```

- `command` — run from the skill directory. Required.
- `requires_env` — env vars that must be set; until then the channel stays
  idle (`missing_env`), not failed. Env vars come from the container
  (`docker run -e ...`), so ask the owner to add secrets there; never write
  tokens into the skill files.
- `owner` / `owner_env` — the owner's user ID on that platform (literal, or
  the env var holding it). One is required. Messages whose `sender` differs
  are dropped by the core.
- `format` — how the platform shows text: `markdown` (sent as-is, default),
  `plain` (markdown flattened), `telegram_html`.
- `max_message_len` — longer answers are split. Default 4000.
- `can_edit` — the platform can edit and delete sent messages: enables the
  live status message ("🔧 Running: go build..."). Without it the core only
  sends `typing`.
- `buttons` — `ask` requests show options as buttons whose `value` comes
  back as the reply. Without it the core lists the options as text.
- `enabled` — `false` keeps the skill without running it.

## Protocol

One JSON object per line, UTF-8. **stdout is for protocol lines only** — log to
stderr (the core keeps the last 100 lines, see `channel_logs`).

The bridge gets these env vars besides the container's: `GOGOGOT_CHANNEL`
(its name) and `GOGOGOT_CHANNEL_INBOX` (a directory for downloaded files).

### Bridge → core: events

Once connected to the platform:

```json
{"event": "ready", "owner_chat": "123456"}
```

`owner_chat` (optional) is where owner notifications go — scheduled task
results, alerts. Without it the core uses the chat the owner last wrote from,
then the owner ID.

For every message the owner sends:

```json
{"event": "message", "chat": "123456", "sender": "123456", "text": "hi",
 "files": [{"path": "<inbox>/abc-photo.jpg", "name": "photo.jpg", "mime": "image/jpeg"}]}
```

- `chat` — where replies go; `sender` — who wrote it (checked against owner).
  Both are strings.
- Slash commands (`/new`, `/stop`, `/history`, `/help`...) are just text; the
  core handles them.
- Button presses are messages too: send the button's value as `text`.
- `files` must be inside `GOGOGOT_CHANNEL_INBOX`; the core reads and deletes
  them. Use mime `text/plain` for text files, they are inlined into the
  message. zip and tar.gz archives are unpacked by the core.

### Core → bridge: requests

Every request has an `id`; answer each with exactly one response line, in any
order (handle slow ones, like uploads, in a thread):

```json
{"id": 7, "ok": true, "msg_id": "42"}
{"id": 8, "ok": false, "error": "chat not found"}
```

| `op` | fields | the bridge should |
|---|---|---|
| `send` | `chat`, `text`, `plain`, `status` | post a message; return its `msg_id`. `plain: true` means show `text` verbatim (no markup parsing). `status: true` marks the live status message (e.g. send it silently). |
| `edit` | `chat`, `msg_id`, `text`, `plain`, `status` | replace a message's text (only called when `can_edit`) |
| `delete` | `chat`, `msg_id` | delete a message (only called when `can_edit`) |
| `typing` | `chat` | show a typing indicator, or just answer ok |
| `send_file` | `chat`, `path`, `caption` | upload the local file |
| `ask` | `chat`, `text`, `plain`, `options: [{value, label}]` | show the question with one button per option (only called when `buttons`) |

If a formatted `send`/`edit` fails, the core retries once with `plain: true`,
so reject markup the platform cannot parse instead of mangling it.

When stdin closes, exit. On SIGTERM, exit. If the bridge exits or crashes the
core restarts it with backoff (1s, 2s, 4s... up to a minute).

## Workflow

1. Read the platform's bot API docs; find how to receive messages
   (long polling or websocket — no public webhook needed) and the five
   operations above.
2. Create the skill dir with `SKILL.md`, `channel.json`, `bridge.py`.
   Start from `/data/skills/telegram/bridge.py` — it is a complete, small
   example of this protocol.
3. Ask the owner to add the token and their user ID to the container env if
   they are missing. Never ask them to paste secrets into the chat.
4. Run `channel_restart <name>`, then check `channel_list` and `channel_logs
   <name>`. Fix and restart until it is `running`.
5. Ask the owner to send a message through the new channel to confirm.

## Rules

- Never disable or weaken the owner check, and never set `owner` to
  something that is not the owner's ID: the channel gives full control of
  this server.
- Before changing a working channel (including telegram), copy the bridge
  (`bridge.py.bak`) so you can roll back; a broken channel may cut the owner
  off. Do not edit the channel you are currently talking through without the
  owner's go-ahead. (Channels shipped with GoGogot, like telegram, are
  restored automatically if they crash 3 times in a row after start; channels
  you wrote are not.)
- Keep bridges dependency-free when you can; if you need a package, install
  it in the skill dir (e.g. `pip install --target vendor`) and document it in
  `SKILL.md`.
