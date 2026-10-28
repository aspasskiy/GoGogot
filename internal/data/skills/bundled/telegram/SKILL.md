---
name: telegram
description: The Telegram channel — the bridge that connects your Telegram bot to you. Read before changing how Telegram messages are received, downloaded or shown, or when Telegram misbehaves (missing messages, failed uploads, formatting errors).
---

# Telegram channel

This skill is a channel (see the `add-channel` skill for the protocol): the
core runs `bridge.py`, which long-polls the Telegram Bot API and relays
messages. Only the owner (`TELEGRAM_OWNER_ID`) can talk to you; the core
enforces it.

It starts by itself whenever `TELEGRAM_BOT_TOKEN` and `TELEGRAM_OWNER_ID`
are set in the container.

## Files

- `channel.json` — manifest: needs `TELEGRAM_BOT_TOKEN` and
  `TELEGRAM_OWNER_ID`, Telegram HTML format, 4000-char messages, editing and
  inline buttons.
- `bridge.py` — the bridge, Python standard library only.

## What the bridge does

- `getUpdates` long polling for messages and button presses (callback
  queries) from the owner; everyone else is ignored.
- Albums (media groups) are collected for a second and forwarded as one
  message.
- Downloads photos (largest size), documents, audio, voice, video, video
  notes, GIFs and static stickers into `GOGOGOT_CHANNEL_INBOX`. Limits:
  text files 512 KB, images 10 MB, everything else 20 MB (the Bot API cannot
  download more than 20 MB). Text-like documents are sent as `text/plain`.
- Locations, venues, contacts, polls and dice become text like
  `[Location: lat=..., lon=...]`.
- Requests: `send`/`edit` use `parse_mode=HTML` unless `plain`; status
  messages are sent silently; `send_file` picks sendPhoto/Video/Audio/Voice/
  Animation by extension and falls back to sendDocument; `ask` shows inline
  buttons. Rate limits (429) are retried after `retry_after`.
- `TELEGRAM_API_BASE` overrides `https://api.telegram.org` (used by tests, or
  a local Bot API server, which lifts the 20 MB limit).

## Changing it

Copy `bridge.py` to `bridge.py.bak` first, edit, then `channel_restart
telegram` and check `channel_logs telegram`. If you are talking through this
bridge right now, a mistake can cut the conversation, so ask the owner first.

Safety net: if the bridge crashes 3 times in a row right after starting, the
core restores the stock files of this skill and moves yours to `.broken/`
(this shows up in `channel_logs`). It does not catch a bridge that runs but
misbehaves — test with the owner after every change. The owner can also
delete `/data/skills/telegram` and restart the container to get the stock
skill back.

When a new version of GoGogot ships an updated bridge, files you have not
modified are upgraded; files you changed are kept and the new version is put
in `.upstream/` next to them for you to merge.

Update this file with anything you learn about Telegram quirks.
