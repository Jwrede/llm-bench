#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="${LLM_BENCH_DATA_DIR:-/opt/llm-bench/data}"
DATA_REPO="${LLM_BENCH_DATA_REPO:-/opt/llm-bench/data-repo}"
JSONL_FILE="${DATA_DIR}/results.jsonl"

if [ ! -f "$JSONL_FILE" ]; then
    echo "no data file at $JSONL_FILE"
    exit 0
fi

lines=$(wc -l < "$JSONL_FILE")
if [ "$lines" -eq 0 ]; then
    echo "no new data to push"
    exit 0
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

git commit -m "data: ${today} (+${lines} records)"
git push origin main

echo "pushed ${lines} records for ${today}"
