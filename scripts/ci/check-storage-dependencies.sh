#!/usr/bin/env bash
set -euo pipefail

echo "Validando boundaries de dependências de storage conforme ADR 019..."

FORBIDDEN_DIRECT_DEPS=(
  "github.com/mattn/go-sqlite3"
  "modernc.org/sqlite"
  "gorm.io/gorm"
)

for pkg in "${FORBIDDEN_DIRECT_DEPS[@]}"; do
  if grep -F "$pkg" go.mod | grep -qv "// indirect"; then
    echo "Dependência estrutural não autorizada detectada: $pkg. Consulte ADR 019 antes de alterar o baseline de storage." >&2
    exit 1
  fi
done

echo "Validação de boundaries de storage concluída com sucesso."
