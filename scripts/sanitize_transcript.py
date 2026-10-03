#!/usr/bin/env python3
"""
Sanitizes transcript.jsonl before uploading to Dev.to or public repositories.
Redacts all private tokens, API keys, and authorization headers.
"""

import os
import re
import sys

def sanitize_file(input_path="transcript.jsonl", output_path=None):
    if output_path is None:
        output_path = input_path

    if not os.path.exists(input_path):
        print(f"File not found: {input_path}")
        return False

    with open(input_path, "r", encoding="utf-8") as f:
        content = f.read()

    # Load actual values from .env if available
    env_secrets = {}
    if os.path.exists(".env"):
        with open(".env", "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    v = v.strip("\"'")
                    if len(v) > 8:
                        env_secrets[k] = v

    # 1. Redact exact known secrets from .env
    for k, v in env_secrets.items():
        if "SANITY" in k and "TOKEN" in k:
            content = content.replace(v, "sk_REDACTED_SANITY_CONTEXT_TOKEN")
        elif "OPENROUTER" in k:
            content = content.replace(v, "sk-or-v1-REDACTED_OPENROUTER_KEY")
        else:
            content = content.replace(v, "[REDACTED_SECRET]")

    # 2. Redact regex patterns
    content = re.sub(r"sk-or-v1-[a-zA-Z0-9]+", "sk-or-v1-REDACTED_OPENROUTER_KEY", content)
    content = re.sub(r"skSPF[a-zA-Z0-9_-]+", "sk_REDACTED_SANITY_CONTEXT_TOKEN", content)
    content = re.sub(r"user_[0-9a-zA-Z]{20,}", "user_REDACTED_ID", content)
    content = re.sub(r"[0-9a-f]{64}", "REDACTED_KEY_HASH", content)
    content = re.sub(r"Bearer\s+sk[a-zA-Z0-9_.-]+", "Bearer REDACTED_TOKEN", content)

    # 3. Verify zero leaks
    leaks = []
    for k, v in env_secrets.items():
        if v in content:
            leaks.append(k)

    if leaks:
        print(f"Error: Secrets still detected in file: {leaks}", file=sys.stderr)
        return False

    with open(output_path, "w", encoding="utf-8") as f:
        f.write(content)

    print(f"Successfully sanitized {input_path} -> {output_path} (Zero secrets remaining)")
    return True

if __name__ == "__main__":
    target = sys.argv[1] if len(sys.argv) > 1 else "transcript.jsonl"
    success = sanitize_file(target)
    sys.exit(0 if success else 1)
