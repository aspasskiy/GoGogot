![GoGogot](https://octagon-lab.sfo3.cdn.digitaloceanspaces.com/gogogot.jpg)

# GoGogot — Lightweight OpenClaw Written in Go

[![Go Version](https://img.shields.io/github/go-mod/go-version/aspasskiy/GoGogot?style=flat-square)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot?style=flat-square)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat-square)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/core-~5%2C500_lines-blue?style=flat-square)](#)
[![Docker](https://img.shields.io/docker/pulls/octagonlab/gogogot?style=flat-square)](#quick-start)

A **lightweight, extensible, and secure** open-source AI agent that lives on your server. It runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks — a self-hosted alternative to OpenClaw (Claude Code) in ~5,500 lines of core Go.

- **Single binary, ~15 MB, ~10 MB RAM** — deploys with one `docker run` command
- **Your keys stay on your server** — no cloud account, no telemetry, no phoning home
- **You pick the model** — Anthropic, OpenAI, or any [OpenRouter](https://openrouter.ai) model
- **Extensible** — clean Go interfaces (`Backend`, `Transport`, `Tool`) make it trivial to add providers, transports, or custom tools

## Quick Start

### Prerequisites

- Get a `TELEGRAM_BOT_TOKEN` by creating a new bot via [@BotFather](https://t.me/BotFather) on Telegram.
- Find your `TELEGRAM_OWNER_ID` (your personal Telegram user ID) using a bot like [@userinfobot](https://t.me/userinfobot). **This is critical for security** — it ensures only you can communicate with your agent.

### Docker

No git clone needed — the image is published on Docker Hub:

```bash
docker run -d --restart unless-stopped \
  --name gogogot \
  -e TELEGRAM_BOT_TOKEN=... \
  -e TELEGRAM_OWNER_ID=... \
  -e GOGOGOT_PROVIDER=anthropic \
  -e ANTHROPIC_API_KEY=... \
  -e GOGOGOT_MODEL=claude-sonnet-4-6 \
  -v ./data:/data \
  -v ./work:/work \
  octagonlab/gogogot:latest
```

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
GOGOGOT_MODEL=claude-sonnet-4-6
EOF

docker compose up -d
```

</details>

<details>
<summary>Local development (without Docker)</summary>

Requires Go 1.25+:

```bash
make generate          # fetch OpenRouter model catalog
go run ./cmd
```

</details>

## Choosing a Model

Set `GOGOGOT_PROVIDER`, `GOGOGOT_MODEL`, and the corresponding API key. The agent will not start without all three.

| Provider | `GOGOGOT_PROVIDER` | API key env | Example `GOGOGOT_MODEL` |
|---|---|---|---|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | `claude-sonnet-4-6`, `claude-opus-4-6`, `claude-haiku-4-5` |
| OpenAI | `openai` | `OPENAI_API_KEY` | `gpt-4o`, `gpt-4.1`, `gpt-5.4`, `o3`, `o4-mini` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | `deepseek/deepseek-v3.2`, `google/gemini-3-flash-preview` |

Model metadata (context window, vision support, pricing) is stored in JSON catalogs under [`llm/catalog/`](internal/llm/catalog/) — just edit the JSON to add or update models.

With OpenRouter you can also pass any slug directly, e.g. `GOGOGOT_MODEL=moonshotai/kimi-k2.5`.

### Short Aliases

For convenience, short aliases are supported as `GOGOGOT_MODEL` values:

| Alias | Resolves to |
|---|---|
| `claude` | `claude-sonnet-4-6` |
| `openai` | `openai/gpt-5-nano` |
| `deepseek` | `deepseek/deepseek-v3.2` |
| `gemini` | `google/gemini-3-pro-preview` |
| `llama` | `meta-llama/llama-4-maverick` |
| `qwen` | `qwen/qwen3.5-397b-a17b` |
| `minimax` | `minimax/minimax-m2.5` |
| `kimi` | `moonshotai/kimi-k2.5` |

Browse all available models: [Anthropic](https://docs.anthropic.com/en/docs/about-claude/models) | [OpenAI](https://platform.openai.com/docs/models) | [OpenRouter](https://openrouter.ai/models) | Benchmarks: [PinchBench](https://pinchbench.com/)

## Features

27 built-in tools across 10 categories:

- **Telegram** — multi-chat, attachments, typing indicators
- **System** — bash, read/write/edit files, system info
- **Web** — Brave search, fetch pages, HTTP requests, downloads
- **Identity** — persistent `soul.md` / `user.md`, auto-evolving
- **Memory** — persistent markdown notes the agent manages itself
- **Skills** — reusable procedural knowledge
- **Task planning** — session-scoped checklist for multi-step work
- **Scheduling** — cron-based self-scheduling, persisted across restarts
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

- Adding a new LLM backend (implement one `Backend` interface method)
- Adding a new transport like Discord or Slack (implement `Channel` + `Replier` — 3 + 6 methods)
- Adding custom models by editing JSON catalogs in [`llm/catalog/`](internal/llm/catalog/)

## License

MIT
