#!/usr/bin/env python3
"""Telegram channel bridge for GoGogot.

Connects a Telegram bot to the agent using the channel protocol: JSON lines,
requests from the core on stdin, responses and events on stdout, logs on
stderr. Standard library only, so it runs anywhere python3 does.

The core renders everything (status, answers, buttons) and checks that the
sender is the owner; this bridge only moves messages between the Bot API and
the core.
"""

import json
import os
import ssl
import sys
import threading
import time
import traceback
import urllib.error
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor

API_BASE = os.environ.get("TELEGRAM_API_BASE", "https://api.telegram.org").rstrip("/")
TOKEN = os.environ.get("TELEGRAM_BOT_TOKEN", "")
OWNER = os.environ.get("TELEGRAM_OWNER_ID", "").strip()
INBOX = os.environ.get("GOGOGOT_CHANNEL_INBOX", "")

POLL_TIMEOUT = 30
MEDIA_GROUP_WAIT = 1.0

MAX_TEXT_FILE = 512 * 1024
MAX_IMAGE_FILE = 10 * 1024 * 1024
MAX_GENERIC_FILE = 20 * 1024 * 1024

# Mirrors render.Commands in the core, for the bot's command menu.
COMMANDS = [
    ("new", "Start a fresh conversation"),
    ("history", "View past conversations"),
    ("memory", "List memory files"),
    ("soul", "Show agent identity (soul.md)"),
    ("user", "Show user profile (user.md)"),
    ("stop", "Cancel the current task"),
    ("help", "Show available commands"),
]

TEXT_MIMES = {
    "application/json", "application/xml", "application/javascript",
    "application/x-yaml", "application/toml", "application/x-sh",
    "application/csv", "application/sql",
}

try:  # system pythons on macOS ship without CA certificates
    import certifi

    SSL_CONTEXT = ssl.create_default_context(cafile=certifi.where())
except ImportError:
    SSL_CONTEXT = None


def log(*args):
    print(*args, file=sys.stderr, flush=True)


_out_lock = threading.Lock()


def emit(obj):
    line = json.dumps(obj, ensure_ascii=False)
    with _out_lock:
        sys.stdout.write(line + "\n")
        sys.stdout.flush()


# --- Bot API ---------------------------------------------------------------


class APIError(Exception):
    pass


def _multipart(fields, files):
    boundary = uuid.uuid4().hex
    parts = []
    for name, value in fields.items():
        parts.append(
            f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n'.encode()
        )
        parts.append(str(value).encode())
        parts.append(b"\r\n")
    for name, (filename, data, mime) in files.items():
        filename = filename.replace('"', "'").replace("\r", "").replace("\n", "")
        parts.append(
            (
                f"--{boundary}\r\n"
                f'Content-Disposition: form-data; name="{name}"; filename="{filename}"\r\n'
                f"Content-Type: {mime}\r\n\r\n"
            ).encode()
        )
        parts.append(data)
        parts.append(b"\r\n")
    parts.append(f"--{boundary}--\r\n".encode())
    return b"".join(parts), f"multipart/form-data; boundary={boundary}"


def api(method, params=None, files=None, timeout=60):
    """Call a Bot API method and return its result. Retries on rate limits."""
    params = {k: v for k, v in (params or {}).items() if v is not None}
    url = f"{API_BASE}/bot{TOKEN}/{method}"
    for attempt in range(4):
        if files:
            fields = {k: json.dumps(v) if isinstance(v, (dict, list)) else v for k, v in params.items()}
            body, ctype = _multipart(fields, files)
        else:
            body, ctype = json.dumps(params).encode(), "application/json"
        req = urllib.request.Request(url, data=body, headers={"Content-Type": ctype})
        try:
            with urllib.request.urlopen(req, timeout=timeout, context=SSL_CONTEXT) as resp:
                data = json.load(resp)
        except urllib.error.HTTPError as e:
            try:
                data = json.load(e)
            except Exception:
                raise APIError(f"{method}: HTTP {e.code}") from None
        if data.get("ok"):
            return data.get("result")
        retry_after = (data.get("parameters") or {}).get("retry_after")
        if data.get("error_code") == 429 and retry_after and attempt < 3:
            time.sleep(min(float(retry_after), 30))
            continue
        raise APIError(f"{method}: {data.get('description') or 'request failed'}")
    raise APIError(f"{method}: rate limited")


def download(file_id, size, limit, name, mime):
    if size and size > limit:
        raise APIError(f"file too large ({size} bytes)")
    info = api("getFile", {"file_id": file_id})
    url = f"{API_BASE}/file/bot{TOKEN}/{info['file_path']}"
    with urllib.request.urlopen(url, timeout=120, context=SSL_CONTEXT) as resp:
        data = resp.read(limit + 1)
    if len(data) > limit:
        raise APIError(f"file too large (over {limit} bytes)")
    path = os.path.join(INBOX, f"{uuid.uuid4().hex}-{os.path.basename(name)}")
    with open(path, "wb") as f:
        f.write(data)
    return {"path": path, "name": name, "mime": mime}


# --- requests from the core --------------------------------------------------


def _parse_mode(req):
    return None if req.get("plain") else "HTML"


def op_send(req):
    msg = api("sendMessage", {
        "chat_id": req["chat"],
        "text": req["text"],
        "parse_mode": _parse_mode(req),
        "disable_notification": True if req.get("status") else None,
    })
    return {"msg_id": str(msg["message_id"])}


def op_edit(req):
    try:
        api("editMessageText", {
            "chat_id": req["chat"],
            "message_id": int(req["msg_id"]),
            "text": req["text"],
            "parse_mode": _parse_mode(req),
        })
    except APIError as e:
        if "message is not modified" not in str(e):
            raise


def op_delete(req):
    api("deleteMessage", {"chat_id": req["chat"], "message_id": int(req["msg_id"])})


def op_typing(req):
    api("sendChatAction", {"chat_id": req["chat"], "action": "typing"})


SEND_BY_EXT = [
    ((".jpg", ".jpeg", ".png", ".webp"), "sendPhoto", "photo"),
    ((".mp4", ".mov", ".avi", ".mkv"), "sendVideo", "video"),
    ((".mp3", ".wav", ".flac", ".aac", ".m4a"), "sendAudio", "audio"),
    ((".ogg", ".opus"), "sendVoice", "voice"),
    ((".gif",), "sendAnimation", "animation"),
]


def op_send_file(req):
    path = req["path"]
    with open(path, "rb") as f:
        data = f.read()
    name = os.path.basename(path)
    method, field = "sendDocument", "document"
    for exts, m, fld in SEND_BY_EXT:
        if path.lower().endswith(exts):
            method, field = m, fld
            break
    params = {"chat_id": req["chat"], "caption": req.get("caption") or None}
    files = {field: (name, data, "application/octet-stream")}
    try:
        api(method, params, files=files, timeout=300)
    except APIError as e:
        if method == "sendDocument":
            raise
        # Photos over 10 MB, odd codecs, ...: a document always goes through.
        log(f"{method} failed ({e}), sending as document")
        api("sendDocument", params, files={"document": (name, data, "application/octet-stream")}, timeout=300)


def op_ask(req):
    options = req.get("options") or []
    buttons = [{"text": o.get("label") or o["value"], "callback_data": o["value"]} for o in options]
    rows = [buttons] if len(buttons) <= 2 else [[b] for b in buttons]
    msg = api("sendMessage", {
        "chat_id": req["chat"],
        "text": req["text"],
        "parse_mode": _parse_mode(req),
        "reply_markup": {"inline_keyboard": rows},
    })
    return {"msg_id": str(msg["message_id"])}


OPS = {
    "send": op_send,
    "edit": op_edit,
    "delete": op_delete,
    "typing": op_typing,
    "send_file": op_send_file,
    "ask": op_ask,
}


def handle_request(req):
    rid = req.get("id")
    op = OPS.get(req.get("op"))
    try:
        if op is None:
            raise APIError(f"unknown op {req.get('op')!r}")
        emit({"id": rid, "ok": True, **(op(req) or {})})
    except Exception as e:
        emit({"id": rid, "ok": False, "error": str(e)})


# --- updates from Telegram -----------------------------------------------------


def is_archive(mime, name):
    lower = name.lower()
    return (
        mime in ("application/zip", "application/x-zip-compressed", "application/gzip", "application/x-gzip")
        or lower.endswith((".zip", ".tar.gz", ".tgz"))
    )


def is_text(mime):
    return mime.startswith("text/") or mime in TEXT_MIMES


def media_of(m):
    """Yield (file_id, size, limit, filename, mime) for every file in a message."""
    if m.get("animation"):
        a = m["animation"]
        yield a["file_id"], a.get("file_size"), MAX_GENERIC_FILE, a.get("file_name") or "animation.mp4", a.get("mime_type") or "video/mp4"
    elif m.get("document"):
        d = m["document"]
        mime, name = d.get("mime_type") or "", d.get("file_name") or "document"
        if is_archive(mime, name):  # unpacked by the core
            yield d["file_id"], d.get("file_size"), MAX_GENERIC_FILE, name, mime
        elif mime.startswith("image/"):
            yield d["file_id"], d.get("file_size"), MAX_IMAGE_FILE, name, mime
        elif is_text(mime) or mime in ("", "application/octet-stream"):
            yield d["file_id"], d.get("file_size"), MAX_TEXT_FILE, name, "text/plain"
        else:
            yield d["file_id"], d.get("file_size"), MAX_GENERIC_FILE, name, mime
    if m.get("photo"):
        p = m["photo"][-1]
        yield p["file_id"], p.get("file_size"), MAX_IMAGE_FILE, "photo.jpg", "image/jpeg"
    if m.get("audio"):
        a = m["audio"]
        yield a["file_id"], a.get("file_size"), MAX_GENERIC_FILE, a.get("file_name") or "audio.mp3", a.get("mime_type") or "audio/mpeg"
    if m.get("voice"):
        v = m["voice"]
        yield v["file_id"], v.get("file_size"), MAX_GENERIC_FILE, "voice.ogg", v.get("mime_type") or "audio/ogg"
    if m.get("video"):
        v = m["video"]
        yield v["file_id"], v.get("file_size"), MAX_GENERIC_FILE, v.get("file_name") or "video.mp4", v.get("mime_type") or "video/mp4"
    if m.get("video_note"):
        v = m["video_note"]
        yield v["file_id"], v.get("file_size"), MAX_GENERIC_FILE, "videonote.mp4", "video/mp4"
    if m.get("sticker") and not m["sticker"].get("is_animated"):
        s = m["sticker"]
        yield s["file_id"], s.get("file_size"), MAX_IMAGE_FILE, "sticker.webp", "image/webp"


def describe_extras(m):
    """Non-file content (places, contacts, polls, dice) as text for the agent."""
    out = []
    if m.get("venue"):
        v = m["venue"]
        loc = v.get("location") or {}
        out.append(f"[Venue: {v.get('title')}, {v.get('address')} — lat={loc.get('latitude', 0):.6f}, lon={loc.get('longitude', 0):.6f}]")
    elif m.get("location"):
        loc = m["location"]
        out.append(f"[Location: lat={loc['latitude']:.6f}, lon={loc['longitude']:.6f}]")
    if m.get("contact"):
        c = m["contact"]
        out.append(f"[Contact: {c.get('first_name', '')} {c.get('last_name', '')}, phone: {c.get('phone_number', '')}]")
    if m.get("poll"):
        p = m["poll"]
        opts = ", ".join(o.get("text", "") for o in p.get("options", []))
        out.append(f"[Poll: {p.get('question')} — options: {opts}]")
    if m.get("dice"):
        d = m["dice"]
        out.append(f"[Dice: {d.get('emoji')} = {d.get('value')}]")
    return out


def forward(msgs):
    """Turn one message (or an album) into a message event for the core."""
    first = msgs[0]
    texts, files = [], []
    for m in msgs:
        text = (m.get("text") or m.get("caption") or "").strip()
        if text:
            texts.append(text)
        for media in media_of(m):
            try:
                files.append(download(*media))
            except Exception as e:
                log(f"failed to process media: {e}")
        texts.extend(describe_extras(m))
    if not texts and not files:
        return
    emit({
        "event": "message",
        "chat": str(first["chat"]["id"]),
        "sender": str(first["from"]["id"]),
        "text": "\n\n".join(texts),
        "files": files,
    })


inbound = ThreadPoolExecutor(max_workers=1)  # keeps messages in order
_groups = {}
_groups_lock = threading.Lock()


def _flush_group(group_id):
    with _groups_lock:
        msgs = _groups.pop(group_id, {}).get("msgs", [])
    if msgs:
        inbound.submit(forward, msgs)


def handle_update(u):
    cb = u.get("callback_query")
    if cb:
        if str(cb["from"]["id"]) != OWNER:
            return
        try:
            api("answerCallbackQuery", {"callback_query_id": cb["id"]})
        except Exception as e:
            log(f"answerCallbackQuery: {e}")
        chat = ((cb.get("message") or {}).get("chat") or {}).get("id", cb["from"]["id"])
        emit({"event": "message", "chat": str(chat), "sender": str(cb["from"]["id"]), "text": cb.get("data", "")})
        return

    m = u.get("message")
    if not m or not m.get("from"):
        return
    if str(m["from"]["id"]) != OWNER:  # the core checks too; this just saves downloads
        return

    group_id = m.get("media_group_id")
    if not group_id:
        inbound.submit(forward, [m])
        return
    # Albums arrive as separate updates: collect them until a second passes
    # without a new one, then forward them as one message.
    with _groups_lock:
        g = _groups.setdefault(group_id, {"msgs": []})
        g["msgs"].append(m)
        if g.get("timer"):
            g["timer"].cancel()
        g["timer"] = threading.Timer(MEDIA_GROUP_WAIT, _flush_group, args=(group_id,))
        g["timer"].daemon = True
        g["timer"].start()


def poll():
    offset = None
    while True:
        try:
            updates = api("getUpdates", {
                "timeout": POLL_TIMEOUT,
                "offset": offset,
                "allowed_updates": ["message", "callback_query"],
            }, timeout=POLL_TIMEOUT + 15)
        except Exception as e:
            log(f"getUpdates: {e}")
            time.sleep(3)
            continue
        for u in updates or []:
            offset = u["update_id"] + 1
            try:
                handle_update(u)
            except Exception:
                log("update failed:\n" + traceback.format_exc())


def main():
    global INBOX
    if not TOKEN or not OWNER:
        log("TELEGRAM_BOT_TOKEN and TELEGRAM_OWNER_ID are required")
        sys.exit(1)
    if not INBOX:
        import tempfile

        INBOX = tempfile.mkdtemp(prefix="telegram-inbox-")
    os.makedirs(INBOX, exist_ok=True)

    try:
        me = api("getMe")  # fails fast on a bad token or no network
    except Exception as e:
        log(f"telegram: cannot reach the Bot API as this bot ({e}); check TELEGRAM_BOT_TOKEN and the network")
        sys.exit(1)
    log(f"telegram: authorized as @{me.get('username')}")
    try:
        api("setMyCommands", {"commands": [{"command": c, "description": d} for c, d in COMMANDS]})
    except Exception as e:
        log(f"setMyCommands: {e}")

    threading.Thread(target=poll, daemon=True).start()
    emit({"event": "ready", "owner_chat": OWNER})
    log(f"telegram: polling for owner {OWNER}")

    requests = ThreadPoolExecutor(max_workers=8)
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except ValueError:
            log(f"bad request line: {line[:200]}")
            continue
        requests.submit(handle_request, req)
    # stdin closed: the core is gone.


if __name__ == "__main__":
    main()
