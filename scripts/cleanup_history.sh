#!/usr/bin/env bash
# 简单示例：清理 N 天之前的历史记录（JSONL）
# 用法: ./scripts/cleanup_history.sh <history_dir> <days>

set -euo pipefail

if [ $# -lt 2 ]; then
  echo "Usage: $0 <history_dir> <keep_days>" >&2
  exit 1
fi

BASE_DIR="$1"
KEEP_DAYS="$2"

if ! [[ $KEEP_DAYS =~ ^[0-9]+$ ]]; then
  echo "keep_days must be integer" >&2
  exit 1
fi

before=$(date -d "-$KEEP_DAYS day" +%Y-%m-%d)

find "$BASE_DIR" -type f -name "*.jsonl" | while read -r file; do
  dir=$(dirname "$file")
  base=$(basename "$file" .jsonl)
  if [[ "$base" < "$before" ]]; then
    rm -f "$file"
  fi
  # 删除空目录
  rmdir --ignore-fail-on-non-empty "$dir" 2>/dev/null || true
done
