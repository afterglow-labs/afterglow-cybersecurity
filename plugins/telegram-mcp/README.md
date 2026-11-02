# Optional Telegram user MCP connection

This helper brings the browser-chat fork's Telegram login, account, dialog, and message tools into Afterglow. It uses **stdio only** and starts only when an operator adds it under **System → MCP**. It does not start an HTTP login server.

The Docker image includes the helper and Python dependencies. For a standalone installation, install `requirements.txt` in a private Python environment. Set `TELEGRAM_API_ID` and `TELEGRAM_API_HASH` to credentials from your Telegram application. Set `TELEGRAM_ALLOWED_CHATS` to a comma-separated list of exact chat IDs or usernames that the agent may message. Sending is denied if the list is empty. You may set `TELEGRAM_MCP_DATA` to a private session directory; the standalone default is `~/.local/share/afterglow-telegram-mcp`.

Add a stdio MCP server with command `python3` and argument `/app/plugins/telegram-mcp/server.py` in Docker, or the absolute plugin path for a standalone installation. Supply the environment variables to the Afterglow process that launches the MCP server. The login tools send a code, verify it, and handle Telegram 2FA. Do not put API credentials or a session file in the task prompt or repository.

The MCP host can invoke the listed tools once connected. Review the host's tool permissions before enabling this optional connection. The Telethon session file is kept under a private directory; back it up and protect it like an account credential.

Derived from `artex-browser-chat/plugins/telegram-mcp` (`13fabbb`), with embedded API credentials and the unauthenticated HTTP wizard removed.
