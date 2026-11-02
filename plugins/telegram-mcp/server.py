#!/usr/bin/env python3
"""Telegram user-client MCP plugin (Telethon).

Login: telegram_login_send_code → telegram_login_verify_code → telegram_login_2fa.
Runs only over local stdio after the operator configures the MCP connection.
"""

from __future__ import annotations

import core
from mcp.server import MCPServer

mcp = MCPServer(
    name="telegram",
    instructions=(
        "Telegram user account plugin. Login with telegram_login_send_code(phone), "
        "then telegram_login_verify_code(code). If status is need_2fa, call "
        "telegram_login_2fa(password). After logged_in, use telegram_me / "
        "telegram_dialogs / telegram_send_message."
    ),
)


@mcp.tool()
async def telegram_status() -> str:
    """Check whether the Telegram user session is logged in."""
    return core.dumps(await core.status())


@mcp.tool()
async def telegram_login_send_code(phone: str) -> str:
    """Start login: send login code to the phone number (E.164, e.g. +9198...)."""
    return core.dumps(await core.login_send_code(phone))


@mcp.tool()
async def telegram_login_verify_code(code: str) -> str:
    """Submit the login code received on Telegram/SMS. May require 2FA next."""
    return core.dumps(await core.login_verify_code(code))


@mcp.tool()
async def telegram_login_2fa(password: str) -> str:
    """Submit Telegram 2FA / cloud password after verify_code returned need_2fa."""
    return core.dumps(await core.login_2fa(password))


@mcp.tool()
async def telegram_logout() -> str:
    """Log out and delete the local Telethon session file."""
    return core.dumps(await core.logout())


@mcp.tool()
async def telegram_me() -> str:
    """Return the logged-in Telegram account profile."""
    return core.dumps(await core.me())


@mcp.tool()
async def telegram_dialogs(limit: int = 20) -> str:
    """List recent chats/dialogs (requires login)."""
    return core.dumps(await core.dialogs(limit))


@mcp.tool()
async def telegram_send_message(chat: str, text: str) -> str:
    """Send a message to a chat (username, phone, or numeric chat id). Requires login."""
    return core.dumps(await core.send_message(chat, text))


def main() -> None:
    mcp.run()


if __name__ == "__main__":
    main()
