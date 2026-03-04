![GoGogot](https://octagon-lab.sfo3.cdn.digitaloceanspaces.com/gogogot.jpg)

# GoGogot — Lightweight OpenClaw Written in Go

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/lines%20of%20code-~4500-blue)](#)
[![Docker](https://img.shields.io/badge/deploy-docker%20compose-2496ED?logo=docker&logoColor=white)](#deployment)

An open-source, self-hosted personal AI agent written in Go. Deploy on your own server as a single ~15 MB binary — it runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks. A lightweight alternative to OpenClaw (Claude Code) in ~4,500 lines of Go. No frameworks, no plugins, no magic.

## What is GoGogot?

GoGogot is a **lightweight, extensible, and secure** open-source AI agent that lives on your server. It can execute shell commands, read and write files, search and browse the web, maintain persistent memory, and run scheduled tasks. The entire agent is a single Go binary under 15 MB that idles at ~10 MB RAM and deploys with one `docker compose up` command.

### Philosophy

The core philosophy of GoGogot is built around being **lightweight, extensible, and secure**:

- **Lightweight & Containerized**: A single Go binary running inside a Docker container. No heavy frameworks, no complex orchestration. Just a simple eval loop with good tools and smart prompts that consistently outperforms complex frameworks.
- **Secure**: You are fully in control. API keys never leave your server, and the agent runs isolated in a container.
- **Single Model & Cost-Efficiency**: Driven by a single LLM of your choice. You can easily switch to affordable Chinese models (like DeepSeek, Qwen, or MiniMax via OpenRouter) to save costs on routine tasks without sacrificing capability.
- **Extensible**: Clean Go interfaces make it trivial to add new LLM providers, transports, or custom tools.

## Use Cases

| Use Case | Example Prompt |
| --- | --- |
| **Daily Digest** | Find top 5 AI news from today, summarize each in 2 sentences, send me every morning at 9:00 |
| **Report Generation** | Download sales data from this URL, calculate totals by region, generate a PDF report |
| **File Processing** | Take these 12 screenshots, merge them into a single PDF, and send the file back |
| **Market Research** | Search the web for pricing of competitors X, Y, Z and make a comparison table |
| **Server Monitoring** | Check disk and memory usage every hour, alert me in Telegram if anything exceeds 80% |
| **Data Extraction** | Fetch this webpage, extract all email addresses and phone numbers into a CSV |
| **Routine Automation** | Every Friday at 18:00, pull this week's git commits and send me a changelog summary |

## GoGogot vs OpenClaw (Claude Code) vs Nanobot


|                    | OpenClaw                               | Nanobot                     | GoGogot                |
| ------------------ | -------------------------------------- | --------------------------- | ---------------------- |
| Language           | TypeScript                             | Python                      | Go                     |
| Codebase           | ~430,000 LOC                           | ~4,000 LOC                  | ~4,500 LOC             |
| Dependencies       | 800+ npm packages                      | 30+ pip packages            | 7                      |
| Install size       | ~1 GB (Node.js + npm)                  | ~150 MB (Python + pip)      | ~15 MB (single binary) |
| RAM idle / working | ~450 MB / 2–8 GB (known memory leaks)  | ~100 MB / ~300 MB           | ~10 MB / ~30 MB        |
| Min requirements   | 2 GB RAM, 2 CPU cores                  | 512 MB RAM                  | Anything with Linux    |
| Deploy             | CLI wizard + daemon + Node >= 22       | `pip install` + config      | `docker compose up`    |
| Best for           | Feature completeness                   | Learning / research         | Production use         |


The bottleneck is always the LLM API call, not the agent runtime. GoGogot compiles to a single binary, runs on a $4/month VPS, and stays out of the way between requests.

## You Are In Control

Everything is configured explicitly via environment variables passed at deploy time. API keys never leave your server — there is no cloud account, no SaaS dashboard, no telemetry, no phoning home.


| Variable             | Purpose                                           |
| -------------------- | ------------------------------------------------- |
| `ANTHROPIC_API_KEY`  | Claude (direct API)                               |
| `OPENROUTER_API_KEY` | DeepSeek, Gemini, MiniMax, Qwen, Llama via OpenRouter |
| `GOGOGOT_MODEL`      | Model ID — see table below (default: first available) |
| `TELEGRAM_BOT_TOKEN` | Your Telegram bot                                 |
| `TELEGRAM_OWNER_ID`  | Only this user can talk to the bot                |
| `BRAVE_API_KEY`      | Web search (optional)                             |


## Choosing a Model

Set the model via environment variable or CLI flag:

```bash
# Environment variable
GOGOGOT_MODEL=deepseek

# CLI flag (overrides env)
./gogogot --model=gemini
```

If `GOGOGOT_MODEL` is not set, the first available provider is used.

### Built-in Models

| ID         | Model                | Provider   | Context | Vision | Notes                        |
| ---------- | -------------------- | ---------- | ------- | ------ | ---------------------------- |
| `claude`   | Claude Sonnet 4.6    | Anthropic  | 200K    | Yes    | Best reasoning, highest cost |
| `deepseek` | DeepSeek V3-0324     | OpenRouter | 128K    | No     | Best cost/quality ratio      |
| `gemini`   | Gemini 2.5 Pro       | OpenRouter | 1M      | Yes    | Largest context window       |
| `minimax`  | MiniMax M2.5         | OpenRouter | 1M      | No     | Cheap, good for routines     |
| `qwen`     | Qwen3 235B A22B      | OpenRouter | 128K    | No     | Strong multilingual          |
| `llama`    | Llama 4 Maverick     | OpenRouter | 1M      | Yes    | Open-source, good all-around |

### Adding Custom Models

Models are defined in `models.json`. Defaults are compiled into the binary, but you can override them by placing your own `models.json` in the data directory (`~/.gogogot/models.json`):

```json
[
  {
    "id": "mythomax",
    "label": "MythoMax 13B (OpenRouter)",
    "model": "gryphe/mythomax-l2-13b",
    "base_url": "https://openrouter.ai/api/v1",
    "api_key_env": "OPENROUTER_API_KEY",
    "format": "openai",
    "context_window": 4096
  }
]
```

Copy an entry, change 3 fields, restart. No recompilation needed. The `api_key_env` field references the environment variable name — keys stay in `.env`, the config is safe to commit.

## Cost

You pick the price/quality tradeoff. All models work out of the box — switch with one env var.


| Model                         | Input (per 1M tokens) | Output (per 1M tokens) |
| ----------------------------- | --------------------- | ---------------------- |
| Qwen3 235B A22B               | $0.14                 | $0.30                  |
| Llama 4 Maverick              | $0.19                 | $0.81                  |
| DeepSeek V3-0324              | Free / $0.28          | Free / $1.10           |
| MiniMax M2.5 (via OpenRouter) | $0.30                 | $1.10                  |
| Gemini 2.5 Pro                | $1.25                 | $10.00                 |
| Claude Sonnet 4.6             | $5.00                 | $25.00                 |


A typical agent session (~50K input, ~10K output):


| Model              | Cost per session |
| ------------------ | ---------------- |
| Qwen3 / DeepSeek   | ~$0.02           |
| MiniMax            | ~$0.03           |
| Gemini 2.5 Pro     | ~$0.16           |
| Claude Sonnet 4.6  | ~$0.50           |


For routine tasks — daily digests, file management, web lookups — DeepSeek or MiniMax are more than enough. Switch to Claude or Gemini for complex reasoning when you need it.

## Extensible by Design

LLM providers and chat transports are clean Go interfaces. Adding a new model = editing a JSON file. Adding a new transport = implementing a few methods. No plugin system, no registry, no framework.

**LLM provider** — 3 methods:

```go
type LLM interface {
    Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error)
    ModelLabel() string
    ContextWindow() int
}
```

Ships with: **Anthropic** (native SDK) and **OpenAI-compatible** (OpenRouter — DeepSeek, Gemini, MiniMax, Qwen, Llama, etc.). Models configured via `models.json` — add any OpenRouter model without recompilation.

**Transport** — 3 methods:

```go
type Transport interface {
    Name() string
    Run(ctx context.Context, handler Handler) error
    SendText(ctx context.Context, channelID string, text string) error
}
```

Optional capabilities via extra interfaces: `FileSender`, `TypingNotifier`, `StatusUpdater`.

Ships with: **Telegram**. Want Discord, Slack, or Matrix? Implement 3 methods and plug it in.

## Features

- **Telegram** — multi-chat, attachments (images, documents), typing indicators
- **System access** — bash, read/write/edit files, system info
- **Web** — search (Brave), fetch pages, HTTP requests, download files
- **Memory** — persistent markdown files the agent reads and writes itself
- **Scheduling** — cron-based self-scheduling, persisted across restarts
- **Compaction** — automatic context compression when approaching token limits
- **Multi-model** — 6 built-in models (Claude, DeepSeek, Gemini, MiniMax, Qwen, Llama), add your own via `models.json`
- **Observability** — structured event system (LLM calls, tool executions, errors)

## Architecture

```
┌──────────┐     ┌────────┐     ┌───────┐     ┌─────┐
│ Telegram │────▸│ Bridge │────▸│ Agent │────▸│ LLM │
└──────────┘     └────────┘     └───┬───┘     └─────┘
                                    │
                                    ▼
                               ┌─────────┐
                               │  Tools  │
                               └─────────┘
                          bash, files, web,
                        memory, scheduler, ...
```

The entire agent is one loop:

```go
for {
    guardrails.Check()
    maybeCompact()

    response := llm.Call(session.Messages())

    if no tool calls in response {
        break // done, send final text to user
    }

    for each tool call {
        result := tools.Execute(name, input)
        session.Append(result)
    }
}
```

The LLM decides everything — when to plan, when to ask the user, when to self-correct. These are prompt strategies, not code constructs. Adding a new behavior = editing the system prompt.

## Tools

17 built-in tools across 5 categories:

- **System** — `bash`, `read_file`, `write_file`, `edit_file`, `list_files`, `system_info`
- **Web** — `web_search` (Brave), `web_fetch`, `web_request`, `web_download`
- **Memory** — `memory_list`, `memory_read`, `memory_write`
- **Scheduling** — `schedule_add`, `schedule_list`, `schedule_remove`
- **Transport** — `send_file`



## Installation & Quick Start

### Prerequisites

- Get a `TELEGRAM_BOT_TOKEN` by creating a new bot via [@BotFather](https://t.me/BotFather) on Telegram.
- Find your `TELEGRAM_OWNER_ID` (your personal Telegram user ID) using a bot like [@userinfobot](https://t.me/userinfobot). **This is critical for security** — it ensures only you can communicate with your agent.

### Docker Deployment

```bash
git clone https://github.com/aspasskiy/GoGogot.git
cd GoGogot

# Configure
cp .env.example .env
# Edit .env: set TELEGRAM_BOT_TOKEN, TELEGRAM_OWNER_ID, API keys

# Run
docker compose -f deploy/docker-compose.yml up -d
```

The Docker image ships with a full Ubuntu environment: bash, git, Python, Node.js, ripgrep, sqlite, postgresql-client, and more.

### Local Development

To run the agent locally without Docker (requires Go 1.25+):

```bash
go run cmd/main.go
```

## Minimum Dependencies

- **Go 1.25** — compiled, concurrent, zero-dependency runtime
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — native Claude API
- [openai-go](https://github.com/openai/openai-go) — OpenRouter / OpenAI-compatible providers
- [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) — Telegram transport
- [robfig/cron](https://github.com/robfig/cron) — scheduler
- [uber/fx](https://github.com/uber-go/fx) — dependency injection
- [goquery](https://github.com/PuerkitoBio/goquery) — HTML parsing for web tools

## License

MIT