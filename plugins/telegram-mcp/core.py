"""Private Telethon session and login helpers for the optional stdio MCP server."""

from __future__ import annotations

import asyncio
import json
import os
import re
import stat
from pathlib import Path
from typing import Any

from telethon import TelegramClient
from telethon.errors import (
    PhoneCodeExpiredError,
    PhoneCodeInvalidError,
    SessionPasswordNeededError,
)

API_ID_RAW = os.environ.get("TELEGRAM_API_ID", "")
API_HASH = os.environ.get("TELEGRAM_API_HASH", "")

DATA_DIR = Path(
    os.environ.get(
        "TELEGRAM_MCP_DATA",
        str(Path.home() / ".local" / "share" / "afterglow-telegram-mcp"),
    )
)
SESSION_PATH = DATA_DIR / "user"

_lock = asyncio.Lock()
_client: TelegramClient | None = None
_phone_code_hash: str | None = None
_pending_phone: str | None = None


def _ensure_data_dir() -> None:
    DATA_DIR.mkdir(mode=0o700, parents=True, exist_ok=True)
    mode = DATA_DIR.lstat().st_mode
    if not stat.S_ISDIR(mode):
        raise RuntimeError("Telegram session directory must be a real directory")
    DATA_DIR.chmod(0o700)
    for suffix in (".session", ".session-journal", ".session-shm", ".session-wal"):
        path = Path(str(SESSION_PATH) + suffix)
        if path.is_symlink():
            raise RuntimeError("Telegram session path must not be a symlink")
        if path.exists():
            path.chmod(0o600)


def _credentials() -> tuple[int, str]:
    if not API_ID_RAW.isdecimal() or not API_HASH:
        raise RuntimeError("Set TELEGRAM_API_ID and TELEGRAM_API_HASH before starting Telegram MCP")
    return int(API_ID_RAW), API_HASH


def _allowed_chat(chat: str) -> bool:
    allowed = {
        item.strip() for item in os.environ.get("TELEGRAM_ALLOWED_CHATS", "").split(",")
        if item.strip()
    }
    return bool(chat and chat in allowed)


async def get_client() -> TelegramClient:
    global _client
    _ensure_data_dir()
    if _client is None:
        api_id, api_hash = _credentials()
        _client = TelegramClient(str(SESSION_PATH), api_id, api_hash)
    if not _client.is_connected():
        await _client.connect()
    return _client


def ok(**kwargs: Any) -> dict[str, Any]:
    return {"ok": True, **kwargs}


def err(message: str, **kwargs: Any) -> dict[str, Any]:
    return {"ok": False, "error": message, **kwargs}


def dumps(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, default=str)


async def status() -> dict[str, Any]:
    async with _lock:
        client = await get_client()
        authorized = await client.is_user_authorized()
        if not authorized:
            return ok(
                status="logged_out",
                next="Enter phone (E.164) to receive a login code",
            )
        me = await client.get_me()
        return ok(
            status="logged_in",
            user={
                "id": me.id,
                "username": me.username,
                "first_name": me.first_name,
                "last_name": me.last_name,
            },
        )


async def login_send_code(phone: str) -> dict[str, Any]:
    global _phone_code_hash, _pending_phone
    phone = (phone or "").strip().replace(" ", "")
    if not re.fullmatch(r"\+[1-9][0-9]{7,14}", phone):
        return err("phone must be E.164 format, e.g. +919876543210")

    async with _lock:
        client = await get_client()
        if await client.is_user_authorized():
            me = await client.get_me()
            return ok(
                status="already_logged_in",
                user={"id": me.id, "username": me.username},
            )
        try:
            result = await client.send_code_request(phone)
        except Exception:  # noqa: BLE001
            return err("send_code failed")
        _phone_code_hash = result.phone_code_hash
        _pending_phone = phone
        return ok(
            status="code_sent",
            phone=phone,
            next="Enter the code from Telegram / SMS",
        )


async def login_verify_code(code: str) -> dict[str, Any]:
    global _phone_code_hash, _pending_phone
    code = (code or "").strip().replace(" ", "")
    if not code:
        return err("code is required")

    async with _lock:
        if not _pending_phone or not _phone_code_hash:
            return err("no pending login — send code to phone first")
        client = await get_client()
        try:
            await client.sign_in(
                phone=_pending_phone, code=code, phone_code_hash=_phone_code_hash
            )
        except SessionPasswordNeededError:
            return ok(
                status="need_2fa",
                next="Account has 2FA. Enter your Telegram cloud password",
            )
        except PhoneCodeInvalidError:
            return err("invalid code")
        except PhoneCodeExpiredError:
            _phone_code_hash = None
            return err("code expired — send a new code")
        except Exception:  # noqa: BLE001
            return err("verify_code failed")

        _phone_code_hash = None
        me = await client.get_me()
        return ok(
            status="logged_in",
            user={
                "id": me.id,
                "username": me.username,
                "first_name": me.first_name,
                "phone": me.phone,
            },
        )


async def login_2fa(password: str) -> dict[str, Any]:
    password = password or ""
    if not password:
        return err("password is required")
    async with _lock:
        client = await get_client()
        try:
            await client.sign_in(password=password)
        except Exception:  # noqa: BLE001
            return err("2fa failed")
        if not await client.is_user_authorized():
            return err("still not authorized after 2fa")
        me = await client.get_me()
        return ok(
            status="logged_in",
            user={
                "id": me.id,
                "username": me.username,
                "first_name": me.first_name,
            },
        )


async def logout() -> dict[str, Any]:
    global _client, _phone_code_hash, _pending_phone
    async with _lock:
        client = await get_client()
        try:
            if await client.is_user_authorized():
                await client.log_out()
            else:
                await client.disconnect()
        except Exception:  # noqa: BLE001
            return err("logout failed")
        finally:
            _client = None
            _phone_code_hash = None
            _pending_phone = None
            for suffix in (".session", ".session-journal", ".session-shm", ".session-wal"):
                p = Path(str(SESSION_PATH) + suffix)
                try:
                    p.unlink(missing_ok=True)
                except OSError:
                    pass
        return ok(status="logged_out")


async def me() -> dict[str, Any]:
    async with _lock:
        client = await get_client()
        if not await client.is_user_authorized():
            return err("not logged in")
        u = await client.get_me()
        return ok(
            user={
                "id": u.id,
                "username": u.username,
                "first_name": u.first_name,
                "last_name": u.last_name,
            }
        )


async def dialogs(limit: int = 20) -> dict[str, Any]:
    limit = max(1, min(int(limit or 20), 100))
    async with _lock:
        client = await get_client()
        if not await client.is_user_authorized():
            return err("not logged in")
        items = []
        async for d in client.iter_dialogs(limit=limit):
            items.append(
                {
                    "id": d.id,
                    "name": d.name,
                    "is_user": d.is_user,
                    "is_group": d.is_group,
                    "is_channel": d.is_channel,
                    "unread": d.unread_count,
                }
            )
        return ok(dialogs=items)


async def send_message(chat: str, text: str) -> dict[str, Any]:
    chat = (chat or "").strip()
    text = text or ""
    if not chat or not text:
        return err("chat and text are required")
    if len(text) > 4000:
        return err("message exceeds 4000 characters")
    if not _allowed_chat(chat):
        return err("chat is not in TELEGRAM_ALLOWED_CHATS")
    async with _lock:
        client = await get_client()
        if not await client.is_user_authorized():
            return err("not logged in")
        entity: Any = int(chat) if chat.lstrip("-").isdigit() else chat
        try:
            msg = await client.send_message(entity, text)
        except Exception:  # noqa: BLE001
            return err("send failed")
        return ok(message_id=msg.id, chat=chat)
