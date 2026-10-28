![GoGogot](https://octagon-lab.sfo3.cdn.digitaloceanspaces.com/gogogot-banner.png)

# GoGogot — Lightweight OpenClaw Written in Go

[![Go Version](https://img.shields.io/github/go-mod/go-version/aspasskiy/GoGogot?style=flat-square)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot?style=flat-square)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat-square)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/core-~11%2C500_lines-blue?style=flat-square)](#)
[![Docker](https://img.shields.io/docker/pulls/octagonlab/gogogot?style=flat-square)](#quick-start)

A **lightweight, extensible, and secure** open-source AI agent that lives on your server. It runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks — a self-hosted alternative to OpenClaw (Claude Code) in ~11,500 lines of Go.

- **Single binary, ~15 MB, ~10 MB RAM** — deploys with one `docker run` command
- **Your keys stay on your server** — no cloud account, no telemetry, no phoning home
- **You pick the model** — Anthropic, OpenAI, or any [OpenRouter](https://openrouter.ai) model
- **Talks wherever you are** — an interactive terminal chat and Telegram out of the box; any other interface plugs in as a [channel skill](#channel-skills) — and the agent can write one for itself (Discord, Slack, email…)
- **Owner-only by design** — the core, not the channel code, checks that every message comes from you and drops the rest
- **Extensible** — small Go interfaces (`Adapter`, `Channel`, `render.Surface`) for providers and transports, JSON catalogs for models

## Quick Start

### Try it in your terminal

No bot needed — run it interactively and chat right in the console:

```bash
docker run -it --rm \
  -e GOGOGOT_PROVIDER=anthropic \
  -e ANTHROPIC_API_KEY=... \
  -e GOGOGOT_MODEL=claude-sonnet-5-5 \
  -v ./data:/data \
  -v ./work:/work \
  octagonlab/gogogot:latest
```

Type a message and press Enter. See [Terminal](#terminal) for the controls.

### Run it on a server with Telegram

#### Prerequisites

- Get a `TELEGRAM_BOT_TOKEN` by creating a new bot via [@BotFather](https://t.me/BotFather) on Telegram.
- Find your `TELEGRAM_OWNER_ID` (your personal Telegram user ID) using a bot like [@userinfobot](https://t.me/userinfobot). **This is critical for security** — it ensures only you can communicate with your agent.

#### Docker

No git clone needed — the image is published on Docker Hub:

```bash
docker run -d --restart unless-stopped \
  --name gogogot \
  -e TELEGRAM_BOT_TOKEN=... \
  -e TELEGRAM_OWNER_ID=... \
  -e GOGOGOT_PROVIDER=anthropic \
  -e ANTHROPIC_API_KEY=... \
  -e GOGOGOT_MODEL=claude-sonnet-5-5 \
  -v ./data:/data \
  -v ./work:/work \
  octagonlab/gogogot:latest
```

Upgrading from an older release? The same command keeps working — no new env vars. A wrong token still stops the container at startup, with the bridge's last log lines in `docker logs`.

The image supports `linux/amd64` and `linux/arm64` and ships with a full Ubuntu environment (bash, git, Python, Node.js, ripgrep, sqlite, postgresql-client, and more).

<details>
<summary>Alternative: Docker Compose</summary>

```bash
curl -O https://raw.githubusercontent.com/aspasskiy/GoGogot/main/deploy/docker-compose.yml

# Create .env with your keys
cat > .env <<EOF
TELEGRAM_BOT_TOKEN=...
TELEGRAM_OWNER_ID=...
GOGOGOT_PROVIDER=anthropic
ANTHROPIC_API_KEY=...
GOGOGOT_MODEL=claude-sonnet-5-5
EOF

docker compose up -d
```

</details>

<details>
<summary>Local development (without Docker)</summary>

Requires Go 1.25+, and `python3` on `PATH` for the Telegram channel (startup fails with a clear error if it is missing):

```bash
make generate          # fetch OpenRouter model catalog
go run ./cmd/gogogot   # opens the terminal chat; also Telegram if TELEGRAM_BOT_TOKEN is set
```

Without Docker the data directory defaults to `~/.gogogot` (override with `GOGOGOT_DATA_DIR`).

</details>

## Channels

A channel is how you talk to the agent. Two ship with GoGogot, and you — or the agent itself — can add more:

| Channel | Kind | Runs when |
|---|---|---|
| **Terminal** | built in (Go) | stdin and stdout are a TTY (`docker run -it`, `go run`) |
| **Telegram** | bundled channel skill (`bridge.py`, Python stdlib only) | `TELEGRAM_BOT_TOKEN` and `TELEGRAM_OWNER_ID` are set |
| **Your own** | channel skill in any language | its `channel.json` says so (required env vars set, not disabled) |

Several channels can run at once: you can start a task in the terminal and get scheduled-task results in Telegram too — owner notifications go to every running channel.

| Env | Default | Meaning |
|---|---|---|
| `GOGOGOT_TRANSPORT` | `auto` | Comma-separated `terminal`, `telegram` or `auto`. `auto` starts the terminal when stdin/stdout are a TTY and Telegram when `TELEGRAM_BOT_TOKEN` is set. Asking for `terminal` without a TTY, or `telegram` without a token, is a startup error |
| `TELEGRAM_API_BASE` | `https://api.telegram.org` | Point the Telegram bridge at a [local Bot API server](https://github.com/tdlib/telegram-bot-api) (lifts the 20 MB download limit) |
| `GOGOGOT_DATA_DIR` | `/data` in Docker, `~/.gogogot` otherwise | Where memory, skills (and so channel skills) and, in terminal mode, the log file live |

Slash commands work the same in every channel: `/new`, `/stop`, `/history`, `/memory`, `/soul`, `/user`, `/help`.

### Terminal

- Type a message and press Enter; `/help` lists the commands
- `/attach <path> [message]` sends a file, optionally with a message
- **Ctrl+C** stops the current task; a second Ctrl+C right after, `/exit` or **Ctrl+D** quits
- The terminal owns the screen, so logs go to `$GOGOGOT_DATA_DIR/gogogot.log` (`./data/gogogot.log` with the command above)

### Channel skills

Any skill directory with a `channel.json` is a channel. The core starts its bridge program as a child process and talks to it in JSON lines over stdin/stdout. **The bridge only moves messages** — the core renders live status, the plan, progress, answers and buttons, splits long messages, runs slash commands, and drops every message whose sender is not the owner. So every channel looks and behaves the same, and a new one is a few hundred lines of script.

```
/data/skills/discord/
  SKILL.md       what the channel is, setup notes
  channel.json   {"command": ["python3", "bridge.py"], "requires_env": ["DISCORD_BOT_TOKEN"],
                  "owner_env": "DISCORD_OWNER_ID", "format": "markdown", "max_message_len": 2000,
                  "can_edit": true, "buttons": false}
  bridge.py      the bridge
```

The bridge reports each message from the platform as an event:

```json
{"event": "message", "chat": "123456", "sender": "123456", "text": "hi", "files": []}
```

and the core asks it to act with requests (`send`, `edit`, `delete`, `typing`, `send_file`, `ask`), each answered with one response line:

```json
{"id": 7, "op": "send", "chat": "123456", "text": "Done ✅", "plain": false, "status": false}
{"id": 7, "ok": true, "msg_id": "42"}
```

The full protocol — manifest fields, the `ready` event, file uploads, error handling, restart backoff — is in [`add-channel/SKILL.md`](internal/data/skills/bundled/add-channel/SKILL.md).

**The agent can connect itself.** Ask it to *"connect yourself to Discord"*: it follows the bundled `add-channel` skill, writes the bridge, and starts it with `channel_restart` — no rebuild, no container restart. Tokens go into the container env (`docker run -e ...`), never into the chat. `channel_list` and `channel_logs` show what is running and why a bridge failed.

**Telegram is a channel skill too** — the bundled [`telegram`](internal/data/skills/bundled/telegram/) skill, so the agent can read, patch and restart its own Telegram bridge. On every start the core installs bundled skills into `$GOGOGOT_DATA_DIR/skills` (`/data/skills` in Docker):

- files are upgraded one by one; a file the agent has modified is kept, and the new version is put next to it in `.upstream/`
- if the agent already has its own skill named `telegram`, the bundled one is installed as `telegram-bundled`
- if a bundled channel crashes 3 times in a row right after starting, its stock files are restored and the broken ones moved to `.broken/`, so a bad edit cannot lock you out (channels the agent wrote itself are not rolled back)

## Choosing a Model

Set `GOGOGOT_PROVIDER`, `GOGOGOT_MODEL`, and the corresponding API key. The agent will not start without all three.

| Provider | `GOGOGOT_PROVIDER` | API key env | Example `GOGOGOT_MODEL` |
|---|---|---|---|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | `claude-sonnet-5-5`, `claude-opus-5-5`, `claude-fable-5-1`, `claude-haiku-5-5` |
| OpenAI | `openai` | `OPENAI_API_KEY` | `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`, `gpt-5.4-mini` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | `qwen/qwen3.7-max`, `deepseek/deepseek-v4-flash`, `x-ai/grok-build-0.1` |

Model metadata (context window, vision support, pricing) is stored in JSON catalogs under [`llm/catalog/`](internal/llm/catalog/) — just edit the JSON to add or update models.

With OpenRouter you can also pass any slug directly, e.g. `GOGOGOT_MODEL=moonshotai/kimi-k2.5`.

### Short Aliases

For convenience, short aliases are supported as `GOGOGOT_MODEL` values:

| Alias | Resolves to | PinchBench avg |
|---|---|---|
| `qwen` | `qwen/qwen3.7-max` | 92.5% |
| `grok` | `x-ai/grok-build-0.1` | 88.9% |
| `deepseek` | `deepseek/deepseek-v4-flash` | 81.7% |
| `gemini` | `google/gemini-3-flash-preview` | 72.1% |
| `kimi` | `moonshotai/kimi-k2.5` | 54.6% |
| `claude` | `claude-sonnet-5-5` | — |
| `openai` | `openai/gpt-5-nano` | — |
| `llama` | `meta-llama/llama-4-maverick` | — |
| `minimax` | `minimax/minimax-m2.5` | — |

`—` means the model is not on the PinchBench leaderboard.

### Benchmarks

[PinchBench](https://pinchbench.com/) runs models through standardized agent tasks in the OpenClaw harness (147 tasks, graded by automated checks and an LLM judge). It measures the *model*, not GoGogot, but it's a good proxy for how well a model drives a tool-calling agent loop like this one. Top 10 (PinchBench v2, snapshot of 2026-10-28):

| # | Model | Avg score | Runs | Avg cost per full run |
|---|---|---|---|---|
| 1 | `anthropic/claude-opus-4.8-fast` | 93.5% | 5 | $165.36 |
| 2 | `qwen/qwen3.7-max` | 92.5% | 6 | $20.51 |
| 3 | `anthropic/claude-opus-4.8` | 90.5% | 5 | $81.79 |
| 4 | `nvidia/nemotron-3.5-bolt-74b-a9b-bf16` | 90.4% | 10 | $0 (reported) |
| 5 | `nvidia/nemotron-3-ultra-550b-a55b` | 89.9% | 5 | $0 (reported) |
| 6 | `xiaomi/mimo-v2.5` | 89.7% | 6 | $6.23 |
| 7 | `x-ai/grok-build-0.1` | 88.9% | 5 | $20.58 |
| 8 | `openai/gpt-5.6-luna` | 88.7% | 5 | $17.14 |
| 9 | `qwen/qwen3.6-flash` | 88.1% | 5 | $14.29 |
| 10 | `nvidia/nemotron-3.5-bolt-74b-a9b-24gb` | 87.8% | 10 | $0 (reported) |

Budget picks: `deepseek/deepseek-v4-flash` (81.7%, ~$1.44 per run) and `stepfun/step-3.5-flash` (79.4%, ~$1.12 per run). Scores are averages over all runs; see [pinchbench.com](https://pinchbench.com/) for the full table of 61 models.

Browse all available models: [Anthropic](https://platform.claude.com/docs/en/about-claude/models/overview) | [OpenAI](https://platform.openai.com/docs/models) | [OpenRouter](https://openrouter.ai/models)

## Features

**37 built-in tools**, plus the core runtime:

- **Channels** — terminal and Telegram out of the box, anything else as a channel skill; attachments, live status, interactive prompts (`ask_user`); `channel_list` / `channel_restart` / `channel_logs` let the agent run and debug its own channels
- **System** — bash, read/write/edit files, regex file search, system info
- **Web** — Brave search, fetch pages, HTTP requests, file downloads
- **Identity** — persistent `soul.md` / `user.md`, auto-evolving
- **Memory** — persistent markdown notes the agent manages itself
- **Recall** — semantic search across past conversations
- **Skills** — reusable procedural knowledge the agent reads and writes
- **Task planning** — session-scoped checklist for multi-step work
- **Scheduling** — cron-based self-scheduling, persisted across restarts; results go to every running channel
- **Compaction** — automatic context compression near token limits
- **Multi-model** — Anthropic, OpenAI, or any OpenRouter model
- **Observability** — compact info-level iteration logs; full request/response dumps at trace level (`LOG_LEVEL=debug`)

## Use Cases

- **Daily digest** — *"Find top 5 AI news, summarize each in 2 sentences, send me every morning at 9:00"*
- **Report generation** — *"Download sales data from this URL, calculate totals by region, generate a PDF report"*
- **File processing** — *"Take these 12 screenshots, merge them into a single PDF, and send the file back"*
- **Market research** — *"Search the web for pricing of competitors X, Y, Z and make a comparison table"*
- **Server monitoring** — *"Check disk and memory usage every hour, alert me if anything exceeds 80%"*
- **Data extraction** — *"Fetch this webpage, extract all email addresses and phone numbers into a CSV"*
- **Routine automation** — *"Every Friday at 18:00, pull this week's git commits and send me a changelog summary"*

## How It Works

The entire agent is a `for` loop. Call the LLM, execute tool calls, feed results back, repeat:

```go
func (a *Agent) Run(ctx context.Context, input []ContentBlock) error {
    a.messages = append(a.messages, userMessage(input))

    for {
        resp, err := a.llm.Call(ctx, a.messages, a.tools)
        if err != nil {
            return err
        }
        a.messages = append(a.messages, resp)

        if len(resp.ToolCalls) == 0 {
            break
        }

        results := a.executeTools(resp.ToolCalls)
        a.messages = append(a.messages, results)
    }
    return nil
}
```

Everything else — memory, scheduling, compaction, identity — is just tools the LLM can call inside this loop.

## Extending

GoGogot is designed to be extended without frameworks or plugin registries:

- **A new LLM backend** — implement the one-method `Adapter` interface
- **A new transport** (Discord, Slack, Matrix, email…) — either a [channel skill](#channel-skills): a bridge script in any language, no Go, no rebuild — or in Go: implement `render.Surface` (7 methods: caps, send, edit, delete, typing, send file, ask) to get the shared UI for free, and `channel.Channel` (3 methods) to plug it into the engine
- **New models** — edit the JSON catalogs in [`llm/catalog/`](internal/llm/catalog/)

## License

MIT
