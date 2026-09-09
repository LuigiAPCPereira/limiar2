#!/usr/bin/env bash
set -euo pipefail

echo "Validando boundaries de dependências de storage conforme ADR 019..."

# ADR 019 aceita github.com/ncruces/go-sqlite3 para o novo storage e permite
# que Tursogo permaneça no caminho legado/importação enquanto necessário.
# ORMs e drivers SQLite estruturais alternativos abaixo não fazem parte da
# baseline aceita e não devem entrar como dependências diretas por acidente.
FORBIDDEN_DIRECT_DEPS=(
  "github.com/mattn/go-sqlite3"
  "modernc.org/sqlite"
  "gorm.io/gorm"
)

# Inspeciona o go.mod diretamente e ignora ocorrências transitivas marcadas
# como indirect. Este gate não tenta provar o escopo de uso do Tursogo; ele
# apenas evita introduzir dependências estruturais fora do baseline aceito.
for pkg in "${FORBIDDEN_DIRECT_DEPS[@]}"; do
  if grep -F "$pkg" go.mod | grep -qv "// indirect"; then
    echo "Dependência estrutural não autorizada detectada: $pkg. Consulte ADR 019 antes de alterar o baseline de storage." >&2
    exit 1
  fi
done

echo "Validação de boundaries de storage concluída com sucesso."
