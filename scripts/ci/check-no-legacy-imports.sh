#!/usr/bin/env bash
set -euo pipefail

if grep -R --include='*.go' -nE '"[^"[:space:]]*/legacy/limiar2(/|\")' .   --exclude-dir=.git --exclude-dir=legacy; then
  echo "ERROR: código Limiar 3 não pode importar legacy/limiar2" >&2
  exit 1
fi

echo "OK: nenhum import L3 -> legacy/limiar2"
