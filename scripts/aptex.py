#!/usr/bin/env python3
"""Independent APTEX server lifecycle and dedicated desktop client."""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request


ROOT = Path(__file__).resolve().parents[1]
PROJECT = "afterglow"  # Preserve the existing PostgreSQL and account volumes.
PROFILE = "local-llm"
SERVICES = ("postgres", "ollama", "scopeweaver")
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def read_env(path: Path) -> dict[str, str]:
    """Read the simple KEY=value entries used by both local Compose files."""
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for line in path.read_text(encoding="utf-8-sig").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue
        key, raw = line.split("=", 1)
        key = key.strip()
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
            continue
        raw = raw.strip()
        if raw.startswith('"') and raw.endswith('"'):
            try:
                value = json.loads(raw)
            except ValueError:
                value = raw[1:-1]
        elif raw.startswith("'") and raw.endswith("'"):
            value = raw[1:-1]
        else:
            value = raw.split(" #", 1)[0].strip()
        values[key] = value
    return values


def atomic_write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".cyber-" + secrets.token_hex(6))
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            stream.write(content)
        os.replace(temporary, path)
        path.chmod(0o600)
    finally:
        temporary.unlink(missing_ok=True)


def update_env(path: Path, updates: dict[str, str]) -> bool:
    """Change only requested keys, retaining every other local setting."""
    original = path.read_text(encoding="utf-8-sig") if path.is_file() else ""
    lines = original.splitlines()
    existing = read_env(path)
    for key, value in updates.items():
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key) or "\n" in value or "\r" in value or "$" in value:
            raise ValueError(f"invalid value for {key}")
        if existing.get(key) == value:
            continue
        literal = json.dumps(value) if any(char in value for char in " #\\\"'") else value
        matching = [i for i, line in enumerate(lines) if re.match(rf"^\s*(?:export\s+)?{re.escape(key)}\s*=", line)]
        if len(matching) > 1:
            raise ValueError(f"{path} has duplicate {key} assignments")
        replacement = f"{key}={literal}"
        if matching:
            lines[matching[0]] = replacement
        else:
            lines.append(replacement)
    result = "\n".join(lines).rstrip("\n") + "\n"
    if result == original:
        return False
    atomic_write(path, result)
    return True


def model_directory(backend: Path, backend_settings: dict[str, str]) -> Path:
    configured = (os.getenv("OLLAMA_MODELS_DIR") or backend_settings.get("OLLAMA_MODELS_DIR"))
    candidates = ([Path(configured).expanduser()] if configured else [
        Path.home() / ".ollama" / "models",
        Path.home() / ".local" / "share" / "afterglow-ollama" / "models",
    ])
    for candidate in candidates:
        if not candidate.is_absolute():
            candidate = backend / candidate
        candidate = candidate.resolve()
        if (candidate / "blobs").is_dir() and (candidate / "manifests").is_dir():
            return candidate
    raise ValueError("OLLAMA_MODELS_DIR must name an existing model directory with blobs/ and manifests/")


def prepare_backend(backend: Path) -> dict[str, str]:
    env_path = backend / ".env"
    fresh = not env_path.is_file()
    if fresh:
        atomic_write(env_path, (backend / ".env.example").read_text(encoding="utf-8"))
    settings = read_env(env_path)
    model_path = model_directory(backend, settings)
    updates = {
        "OLLAMA_MODELS_DIR": str(model_path),
        "SCOPEWEAVER_LLM_PROVIDER": "ollama",
        "SCOPEWEAVER_LLM_BASE_URL": "http://ollama:11434",
    }
    if fresh or "SCOPEWEAVER_LOCAL_LLM_ONLY" not in settings:
        updates["SCOPEWEAVER_LOCAL_LLM_ONLY"] = "true"
    if not settings.get("POSTGRES_PASSWORD"):
        updates["POSTGRES_PASSWORD"] = secrets.token_urlsafe(32)
    if not settings.get("SCOPEWEAVER_LLM_MODEL"):
        updates["SCOPEWEAVER_LLM_MODEL"] = "qwen3:32b"
    if not settings.get("SCOPEWEAVER_LLM_CONTEXT_WINDOW_K"):
        updates["SCOPEWEAVER_LLM_CONTEXT_WINDOW_K"] = "32"
    update_env(env_path, updates)
    for name in ("data", "private/c2-binaries"):
        (backend / name).mkdir(parents=True, exist_ok=True, mode=0o700)
    return read_env(env_path)


def docker_running() -> bool:
    if not shutil.which("docker"):
        raise ValueError("Docker is not installed")
    return subprocess.run(["docker", "info", "--format", "{{.ServerVersion}}"],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0


def compose_command(backend: Path, *args: str) -> list[str]:
    return ["docker", "compose", "-p", PROJECT, "-f", str(backend / "docker-compose.yml"),
            "-f", str(backend / "docker-compose.host-model.yml"), "--profile", PROFILE, *args]


def all_backend_services_running(backend: Path) -> bool:
    result = subprocess.run(compose_command(backend, "ps", "--status", "running", "--services"),
                            cwd=backend, check=True, text=True, capture_output=True)
    return {"postgres", "ollama", "scopeweaver"}.issubset(result.stdout.split())


def wait_for_local_model(backend: Path, model: str) -> None:
    if not re.fullmatch(r"[A-Za-z0-9_.][A-Za-z0-9_.:/-]*", model):
        raise ValueError("SCOPEWEAVER_LLM_MODEL must name one local Ollama model")
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        result = subprocess.run(compose_command(backend, "exec", "-T", "ollama", "ollama", "show", model),
                                cwd=backend, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode == 0:
            return
        time.sleep(1)
    raise ValueError(f"local Ollama model {model} was not found or Ollama did not become ready; no model was downloaded")


def ollama_ip_and_mount(backend: Path, expected_models: Path) -> str:
    container_id = subprocess.run(compose_command(backend, "ps", "-q", "ollama"), cwd=backend,
                                  check=True, text=True, capture_output=True).stdout.strip()
    if not container_id:
        raise ValueError("the APTEX Ollama container did not start")
    container = json.loads(subprocess.run(["docker", "inspect", container_id], check=True,
                                          text=True, capture_output=True).stdout)[0]
    if not container["State"]["Running"] or "OLLAMA_NO_CLOUD=1" not in container["Config"]["Env"]:
        raise ValueError("Ollama must be running with cloud access disabled")
    mount = next((item for item in container["Mounts"] if item["Destination"] == "/models"), None)
    if not mount or mount["RW"] or Path(mount["Source"]).resolve() != expected_models:
        raise ValueError("Ollama must read the verified host model directory read-only")
    if (container["HostConfig"].get("PortBindings") or {}).get("11434/tcp"):
        raise ValueError("Ollama port 11434 must not be published on the host")
    network = container["NetworkSettings"]["Networks"].get(PROJECT + "_default")
    if not network or not network.get("IPAddress"):
        raise ValueError("Ollama has no address on the private APTEX network")
    return str(ipaddress.ip_address(network["IPAddress"]))


def reconcile_cidrs(configured: str, previous: str, current: str) -> str:
    entries = [part.strip() for part in configured.split(",") if part.strip()]
    entries = [part for part in entries if part != previous]
    if current not in entries:
        entries.append(current)
    return ",".join(entries)


def update_local_allowlist(backend: Path, settings: dict[str, str], address: str) -> bool:
    if settings.get("SCOPEWEAVER_LOCAL_LLM_ONLY", "false").lower() != "true":
        return False
    credential_dir = Path.home() / ".config" / "afterglow-security"
    credential_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    credential_dir.chmod(0o700)
    state = credential_dir / "managed-ollama-cidr"
    previous = state.read_text().strip() if state.is_file() else ""
    current = address + ("/32" if ":" not in address else "/128")
    value = reconcile_cidrs(settings.get("SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS", ""), previous, current)
    changed = update_env(backend / ".env", {"SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS": value})
    atomic_write(state, current + "\n")
    return changed


def request(base: str, method: str, path: str, body: dict | None = None, token: str = "") -> tuple[int, dict]:
    headers = {"Accept": "application/json"}
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    outgoing = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        response = OPENER.open(outgoing, timeout=10)
    except urllib.error.HTTPError as exc:
        response = exc
    with response:
        raw = response.read()
        try:
            payload = json.loads(raw) if raw else {}
        except ValueError:
            payload = {}
        return response.status, payload if isinstance(payload, dict) else {}


def wait_for_backend(base: str) -> None:
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            status, _ = request(base, "GET", "/api/health")
            if status == 200:
                return
        except OSError:
            pass
        time.sleep(1)
    raise ValueError("APTEX did not become healthy within 90 seconds")


def admin_password_file() -> Path:
    directory = Path.home() / ".config" / "afterglow-security"
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    directory.chmod(0o700)
    return directory / "afterglow-admin.txt"


def admin_password(path: Path) -> str:
    if not path.is_file():
        raise ValueError(f"APTEX admin password file is missing: {path}")
    for line in path.read_text().splitlines():
        if line.startswith("Afterglow password: "):
            return line.split(": ", 1)[1]
    raise ValueError(f"APTEX admin password file has no password: {path}")


def verify_repository() -> None:
    for name in ("docker-compose.yml", "docker-compose.host-model.yml", ".env.example"):
        if not (ROOT / name).is_file():
            raise ValueError(f"APTEX checkout is missing {ROOT / name}")


def base_url() -> str:
    port = read_env(ROOT / ".env").get("ARTEX_PORT") or "8787"
    if not port.isdecimal() or not 1 <= int(port) <= 65535:
        raise ValueError("ARTEX_PORT must be a valid TCP port")
    return f"http://127.0.0.1:{port}"


def ensure_docker() -> None:
    if docker_running():
        return
    if not shutil.which("systemctl") or not shutil.which("sudo"):
        raise ValueError("Start Docker, then run aptex-start again")
    print("Starting Docker...", flush=True)
    subprocess.run(["sudo", "-n", "systemctl", "start", "docker.service"], check=True)
    for _ in range(30):
        if docker_running():
            return
        time.sleep(1)
    raise ValueError("Docker did not become ready")


def ensure_administrator(base: str) -> None:
    status, state = request(base, "GET", "/api/auth/status")
    if status != 200:
        raise ValueError(f"APTEX authentication status returned HTTP {status}")
    if state.get("initialized"):
        return
    username = state.get("username")
    if not isinstance(username, str) or not username:
        raise ValueError("APTEX authentication status omitted its username")
    path = admin_password_file()
    if path.is_file():
        password = admin_password(path)
    else:
        password = secrets.token_urlsafe(32)
        atomic_write(path, f"Afterglow username: {username}\nAfterglow password: {password}\n")
    status, payload = request(base, "POST", "/api/auth/init", {"password": password})
    if status != 200 or not isinstance(payload.get("token"), str):
        raise ValueError(f"APTEX administrator setup returned HTTP {status}")
    print(f"APTEX administrator created; initial password saved in {path}.", flush=True)


def start(*, rebuild: bool = False) -> None:
    verify_repository()
    settings = prepare_backend(ROOT)
    ensure_docker()
    running = all_backend_services_running(ROOT)
    print("Checking running APTEX services..." if running else "Starting APTEX services...", flush=True)
    build_option = "--build" if rebuild or not running else "--no-build"
    subprocess.run(compose_command(ROOT, "up", "-d", build_option, *SERVICES), cwd=ROOT, check=True)
    address = ollama_ip_and_mount(ROOT, model_directory(ROOT, settings))
    wait_for_local_model(ROOT, settings["SCOPEWEAVER_LLM_MODEL"])
    if update_local_allowlist(ROOT, read_env(ROOT / ".env"), address):
        subprocess.run(compose_command(ROOT, "up", "-d", "--no-deps", "--no-build", "--force-recreate", "scopeweaver"),
                       cwd=ROOT, check=True)
    base = base_url()
    wait_for_backend(base)
    ensure_administrator(base)
    print(f"APTEX server ready: {base}. Open its client with aptex.", flush=True)


def stop() -> None:
    verify_repository()
    # Checking a stopped systemd Docker socket can start the daemon and wake
    # restart-policy containers. Avoid that on the local system Docker context.
    if (sys.platform.startswith("linux") and not os.getenv("DOCKER_HOST")
            and os.getenv("DOCKER_CONTEXT", "default") == "default" and shutil.which("systemctl")):
        result = subprocess.run(["systemctl", "is-active", "--quiet", "docker.service"],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode == 3:
            print("Docker is inactive; APTEX services are already stopped.", flush=True)
            return
    if not docker_running():
        print("APTEX services are already stopped.", flush=True)
        return
    subprocess.run(compose_command(ROOT, "stop", "scopeweaver", "ollama", "postgres"), cwd=ROOT, check=True)
    print("APTEX services stopped; their data is retained.", flush=True)


def client(*, no_open: bool = False) -> None:
    verify_repository()
    base = base_url()
    try:
        status, _ = request(base, "GET", "/api/health")
    except OSError as exc:
        raise ValueError("APTEX is not running. Start it with aptex-start.") from exc
    if status != 200:
        raise ValueError(f"APTEX is not ready (HTTP {status}). Start it with aptex-start.")
    if no_open:
        print(f"APTEX client ready: {base}", flush=True)
        return
    browser = next((path for name in ("google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
                                     "microsoft-edge", "brave-browser") if (path := shutil.which(name))), None)
    if browser is None:
        raise ValueError(f"Install Chrome or Chromium for the APTEX app window, or open {base}")
    profile = Path.home() / ".local" / "share" / "aptex" / "client"
    profile.mkdir(parents=True, exist_ok=True, mode=0o700)
    profile.chmod(0o700)
    subprocess.Popen([browser, f"--app={base}", f"--user-data-dir={profile}", "--class=APTEX", "--new-window"],
                     stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                     start_new_session=True)
    print(f"Opened APTEX client: {base}", flush=True)


def desktop_argument(value: str) -> str:
    if "\n" in value or "\r" in value:
        raise ValueError("desktop launcher path contains a line break")
    for old, new in (("\\", "\\\\"), ('"', '\\"'), ("`", "\\`"), ("$", "\\$"), ("%", "%%")):
        value = value.replace(old, new)
    return '"' + value + '"'


def install() -> None:
    verify_repository()
    script = Path(__file__).resolve(strict=True)
    bin_dir = Path.home() / ".local" / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    links = [bin_dir / name for name in ("aptex", "aptex-start", "aptex-stop")]
    for name in ("odysseus-cyber-start", "odysseus-cyber-stop"):
        path = bin_dir / name
        if path.is_symlink() and path.resolve().name in ("odysseus_cyber_stack.py", "aptex.py"):
            links.append(path)
    for path in links:
        if path.exists() or path.is_symlink():
            if not path.is_symlink() or path.resolve().name not in ("aptex.py", "odysseus_cyber_stack.py"):
                raise ValueError(f"shortcut already exists and is not managed by APTEX: {path}")
        if path.is_symlink() and path.resolve() == script:
            continue
        temporary = path.with_name(path.name + ".aptex-" + secrets.token_hex(6))
        temporary.symlink_to(script)
        os.replace(temporary, path)
    desktop = Path.home() / ".local" / "share" / "applications" / "aptex.desktop"
    marker = "# Managed by APTEX launcher"
    if desktop.exists() and marker not in desktop.read_text():
        raise ValueError(f"desktop launcher already exists and is not managed by APTEX: {desktop}")
    icon = ROOT / "web" / "public" / "afterglow.svg"
    if not icon.is_file():
        raise ValueError(f"APTEX icon is missing: {icon}")
    command = desktop_argument(sys.executable) + " " + desktop_argument(str(script)) + " client"
    atomic_write(desktop, marker + "\n[Desktop Entry]\nType=Application\nName=APTEX\n"
                 "Comment=Autonomous Purple Team Execution\nExec=" + command + "\nIcon=" + str(icon)
                 + "\nTerminal=false\nStartupWMClass=APTEX\nCategories=Development;\n")
    print("Installed aptex, aptex-start, aptex-stop, and the APTEX desktop entry.", flush=True)


def main() -> int:
    name = Path(sys.argv[0]).name
    default = "start" if name.endswith("-start") else "stop" if name.endswith("-stop") else "client"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", nargs="?", choices=("start", "stop", "client", "install"), default=default)
    parser.add_argument("--rebuild", action="store_true", help="rebuild the APTEX image during start")
    parser.add_argument("--no-open", action="store_true", help="check the client URL without opening a window")
    args = parser.parse_args()
    if args.rebuild and args.action != "start":
        parser.error("--rebuild is available only with start")
    if args.no_open and args.action != "client":
        parser.error("--no-open is available only with client")
    try:
        if args.action == "start":
            start(rebuild=args.rebuild)
        elif args.action == "client":
            client(no_open=args.no_open)
        else:
            {"stop": stop, "install": install}[args.action]()
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        print(f"aptex: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
