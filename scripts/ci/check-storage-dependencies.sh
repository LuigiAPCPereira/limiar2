#!/usr/bin/env bash
set -euo pipefail

echo "Validando boundaries de dependências de storage conforme ADR 019..."

if [[ ! -f go.mod ]]; then
  echo "go.mod não encontrado no diretório de trabalho; recusando validar dependencies fora da raiz do módulo." >&2
  exit 1
fi

# ADR 019 aceita github.com/ncruces/go-sqlite3 para o novo storage e permite
# que Tursogo permaneça no caminho legado/importação enquanto necessário.
# ORMs e drivers SQLite estruturais alternativos abaixo não fazem parte da
# baseline aceita e não devem entrar como dependências diretas por acidente.
declare -ar FORBIDDEN_DIRECT_DEPS=(
  "github.com/mattn/go-sqlite3"
  "modernc.org/sqlite"
  "gorm.io/gorm"
)

# Inspeciona require directives do go.mod e rejeita somente o path exato como
# dependência direta. Dependências transitivas marcadas com // indirect seguem
# permitidas. Este gate não tenta provar o escopo de uso do Tursogo; ele apenas
# evita introduzir dependências estruturais fora do baseline aceito.
for pkg in "${FORBIDDEN_DIRECT_DEPS[@]}"; do
  if awk -v pkg="$pkg" '
    ($1 == pkg || ($1 == "require" && $2 == pkg)) && $0 !~ /\/\/[[:space:]]*indirect[[:space:]]*$/ {
      found = 1
    }
    END { exit(found ? 0 : 1) }
  ' go.mod; then
    echo "Dependência estrutural não autorizada detectada: $pkg. Consulte ADR 019 antes de alterar o baseline de storage." >&2
    exit 1
  fi
done

echo "Validação de boundaries de storage concluída com sucesso."
