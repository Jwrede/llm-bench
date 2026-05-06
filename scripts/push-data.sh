#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="${LLM_BENCH_DATA_DIR:-/opt/llm-bench/data}"
DATA_REPO="${LLM_BENCH_DATA_REPO:-/opt/llm-bench/data-repo}"
JSONL_FILE="${DATA_DIR}/results.jsonl"
ALERT_FILE="${DATA_DIR}/alert.flag"

if [ ! -f "$JSONL_FILE" ]; then
    echo "no data file at $JSONL_FILE"
    exit 0
fi

lines=$(wc -l < "$JSONL_FILE")
if [ "$lines" -eq 0 ]; then
    echo "no new data to push"
    exit 0
fi

errors=$(grep -c '"status":"error"' "$JSONL_FILE" 2>/dev/null || true)
error_pct=0
if [ "$lines" -gt 0 ]; then
    error_pct=$(( errors * 100 / lines ))
fi

if [ "$error_pct" -ge 50 ]; then
    if [ ! -f "$ALERT_FILE" ]; then
        echo "ALERT: ${error_pct}% error rate (${errors}/${lines} probes failed)" >&2
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) error_pct=${error_pct} errors=${errors} total=${lines}" > "$ALERT_FILE"
    fi
elif [ -f "$ALERT_FILE" ]; then
    rm "$ALERT_FILE"
fi

today=$(date -u +%Y-%m-%d)
month=$(date -u +%Y-%m)
target_dir="${DATA_REPO}/data/${month}"
target_file="${target_dir}/${today}.jsonl"

mkdir -p "$target_dir"

cat "$JSONL_FILE" >> "$target_file"

> "$JSONL_FILE"

cd "$DATA_REPO"
git add "data/${month}/${today}.jsonl"

if git diff --cached --quiet; then
    echo "no changes to commit"
    exit 0
fi

git commit -m "data: ${today} (+${lines} records, ${errors} errors)"
git push origin main

echo "pushed ${lines} records for ${today} (${errors} errors, ${error_pct}%)"
