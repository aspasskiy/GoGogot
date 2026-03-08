# GoGogot — Lightweight OpenClaw Written in Go

[![Go Version](https://img.shields.io/github/go-mod/go-version/aspasskiy/GoGogot?style=flat-square)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot?style=flat-square)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat-square)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/lines-~4%2C500-blue?style=flat-square)](#)
[![Docker](https://img.shields.io/docker/pulls/octagonlab/gogogot?style=flat-square)](#quick-start)

A **lightweight, extensible, and secure** open-source AI agent that lives on your server. It runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks — a self-hosted alternative to OpenClaw (Claude Code) in ~4,500 lines of Go.

- **Single binary, ~15 MB, ~10 MB RAM** — deploys with one `docker run` command
- **Your keys stay on your server** — no cloud account, no telemetry, no phoning home
- **You pick the model** — built-in aliases from budget to frontier, or pass any [OpenRouter](https://openrouter.ai) slug directly. Context, vision, pricing auto-populated from catalog at build time
- **Extensible** — clean Go interfaces (`Backend`, `Transport`, `Tool`) make it trivial to add providers, transports, or custom tools. [Details →](docs/extending.md)

### How It Works

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

That's it. Everything else — memory, scheduling, compaction, identity — is just tools the LLM can call inside this loop.

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
  -e OPENROUTER_API_KEY=... \
  -e GOGOGOT_MODEL=deepseek \
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
OPENROUTER_API_KEY=...
GOGOGOT_MODEL=deepseek
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

## Use Cases

> 📰 **Daily Digest**
> *"Find top 5 AI news from today, summarize each in 2 sentences, send me every morning at 9:00"*

> 📊 **Report Generation**
> *"Download sales data from this URL, calculate totals by region, generate a PDF report"*

> 📁 **File Processing**
> *"Take these 12 screenshots, merge them into a single PDF, and send the file back"*

> 🔍 **Market Research**
> *"Search the web for pricing of competitors X, Y, Z and make a comparison table"*

> 🖥️ **Server Monitoring**
> *"Check disk and memory usage every hour, alert me in Telegram if anything exceeds 80%"*

> 🗃️ **Data Extraction**
> *"Fetch this webpage, extract all email addresses and phone numbers into a CSV"*

> ⚙️ **Routine Automation**
> *"Every Friday at 18:00, pull this week's git commits and send me a changelog summary"*

## Features

27 built-in tools across 10 categories:

- **Telegram** — multi-chat, attachments (images, documents), typing indicators
- **System access** — bash, read/write/edit files, system info
- **Web** — search (Brave), fetch pages, HTTP requests, download files
- **Identity** — persistent `soul.md` (agent personality) and `user.md` (owner profile), auto-evolving through conversations
- **Memory** — persistent markdown files the agent reads and writes itself
- **Skills** — reusable procedural knowledge the agent creates and consults
- **Task planning** — session-scoped checklist for multi-step work
- **Scheduling** — cron-based self-scheduling, persisted across restarts
- **Compaction** — automatic context compression when approaching token limits
- **Multi-model** — 7 built-in aliases + any OpenRouter model via slug (context, vision, pricing auto-populated from catalog)
- **Observability** — structured event system (LLM calls, tool executions, errors)

## Choosing a Model

Set the model via `GOGOGOT_MODEL` env var or `--model` CLI flag. Two providers are supported:

**Anthropic** — requires `ANTHROPIC_API_KEY` ([get key](https://console.anthropic.com/)):

```bash
docker run -d --restart unless-stopped \
  --name gogogot \
  -e TELEGRAM_BOT_TOKEN=... \
  -e TELEGRAM_OWNER_ID=... \
  -e ANTHROPIC_API_KEY=... \
  -e GOGOGOT_MODEL=claude \
  -v ./data:/data -v ./work:/work \
  octagonlab/gogogot:latest
```

**OpenRouter** — 200+ models, requires `OPENROUTER_API_KEY` ([get key](https://openrouter.ai/keys)):

```bash
docker run -d --restart unless-stopped \
  --name gogogot \
  -e TELEGRAM_BOT_TOKEN=... \
  -e TELEGRAM_OWNER_ID=... \
  -e OPENROUTER_API_KEY=... \
  -e GOGOGOT_MODEL=deepseek \
  -v ./data:/data -v ./work:/work \
  octagonlab/gogogot:latest
```

Built-in aliases: `claude`, `deepseek`, `gemini`, `minimax`, `qwen`, `llama`, `kimi`. You can also pass any OpenRouter slug directly: `GOGOGOT_MODEL=openai/gpt-5.4`.

Browse all available models: [OpenRouter Models](https://openrouter.ai/models) | Independent benchmarks: [PinchBench](https://pinchbench.com/)

## Extending

GoGogot is designed to be extended without frameworks or plugin registries. See [docs/extending.md](docs/extending.md) for details on:

- Adding a new LLM backend (implement one `Backend` interface method)
- Adding a new transport like Discord or Slack (implement 3 `Transport` methods)
- Adding custom models via OpenRouter slug or code aliases

## License

MIT
