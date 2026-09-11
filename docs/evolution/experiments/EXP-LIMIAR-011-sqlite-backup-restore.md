# EXP-LIMIAR-011 — Backup/restore consistente do novo SQLite

Authority: Non-authoritative
Status: In Progress

## Hipótese

O novo storage SQLite pode produzir um snapshot consistente e restaurável sem copiar diretamente o arquivo principal/WAL, preservando ownership (`application_id`), Evidence já commitada e um ponto no tempo bem definido.

## Evidência externa

A versão usada pelo projeto, `github.com/ncruces/go-sqlite3 v0.35.3`, expõe suporte à Online Backup API. SQLite também oferece `VACUUM INTO` como mecanismo distinto para produzir uma cópia consistente de um banco em um novo arquivo: a Online Backup API copia páginas incrementalmente, enquanto `VACUUM INTO` reescreve o conteúdo lógico em um novo banco compactado.

Este experimento usa `VACUUM INTO` somente para reduzir a incerteza sobre o contrato físico de snapshot/restore. Ele não escolhe ainda a API final de backup de produção. A documentação do SQLite define o argumento de `INTO` como uma expressão SQL escalar que resulta no nome do arquivo; por isso o harness mantém o path como parâmetro em vez de interpolá-lo no SQL.

## Boundary

Este experimento não adiciona API pública, não muda defaults, não altera migrations e não autoriza uma política de backup. Ele exercita somente o banco novo já autorizado pelos ADRs 019 e 020.

O teste cria um storage temporário, persiste uma Evidence, produz o snapshot enquanto o storage de origem permanece aberto, persiste uma segunda Evidence após o snapshot e então abre o arquivo de backup pelo mesmo boundary `Open` de produção. No restaurado, ele compara a linha de Evidence completa e prova que o store continua apto a receber nova Evidence.

## Critérios de suporte

A hipótese pode ser considerada suportada neste boundary quando o teste demonstrar simultaneamente:

1. o snapshot é produzido enquanto a origem está aberta;
2. a origem continua gravável depois do snapshot;
3. o arquivo restaurado preserva o `application_id` do novo storage;
4. `PRAGMA integrity_check` do restaurado retorna `ok`;
5. a Evidence commitada antes do snapshot preserva identidade, metadados, payload e hash;
6. a Evidence gravada depois do snapshot não aparece no restaurado;
7. o restaurado é aceito pelo mesmo `Open` usado pelo novo storage;
8. depois de aberto, o restaurado continua utilizável pelo `EvidenceAppender` de produção.

## Fora de escopo

- retenção e rotação de backups;
- criptografia e armazenamento externo;
- backup do banco legado;
- restore destrutivo sobre um banco ativo;
- integração com CLI/configuração;
- política operacional de shutdown, checkpoints ou cópia de WAL;
- política de permissões do artefato durante o intervalo entre criação do snapshot e abertura pelo boundary `Open`;
- garantia multi-plataforma além dos gates executados nesta etapa.

## Resultado atual

`In Progress`.

O harness foi materializado em `internal/storage/sqlite/backup_experiment_test.go`. O resultado só deve mudar para `Supported` depois que os gates reais executarem o teste no HEAD da PR.

## Próximo gate

Executar os gates Go/CI do projeto. Se o comportamento for suportado, usar a Evidence para decidir se o mecanismo de produção deve usar a Online Backup API do ncruces, `VACUUM INTO` ou outro boundary menor. A decisão de produção também precisa fechar explicitamente a política de permissões do artefato de backup; o experimento atual não transforma esse aspecto operacional em garantia implícita. Não transformar o mecanismo experimental em API permanente sem decisão proporcional ao risco operacional.
