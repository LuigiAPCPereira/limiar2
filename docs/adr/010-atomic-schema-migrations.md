# ADR 010 — Migrações Atômicas via schema_migrations

## Status

Aceito

## Contexto

Originalmente, o esquema do banco de dados era gerenciado por arquivos SQL contendo instruções idempotentes como `CREATE TABLE IF NOT EXISTS`. Estes arquivos eram executados lexicalmente toda vez que o sistema inicializava. Embora simples, essa abordagem inviabiliza migrações de estrutura (como `ALTER TABLE`, `DROP INDEX`, etc.) de forma segura, já que elas não são inerentemente idempotentes. Isso dificultava a evolução do schema em produção de forma confiável.

## Decisão

Adotar o controle transacional de versões para o esquema do banco de dados, registrando cada migração executada na tabela `schema_migrations`.

O fluxo atualizado na inicialização do repositório é:
1. Cria a tabela `schema_migrations (version TEXT PRIMARY KEY)` caso não exista.
2. Lê todos os arquivos `*.sql` da pasta embutida de migrations.
3. Filtra arquivos cuja versão já conste na tabela `schema_migrations`.
4. Para cada arquivo pendente, executa seu conteúdo em uma transação (`BeginTx`). Se o SQL for bem sucedido, um `INSERT` correspondente é feito em `schema_migrations`. Se ocorrer erro, a transação realiza o `Rollback`.


Para bancos legados que já registraram versões antigas removidas do repositório, a abertura do banco também executa uma etapa de compatibilidade condicionada por introspecção (`PRAGMA table_info`) dentro de `internal/storage/migrations.go`. Essa etapa só adiciona colunas ausentes e índices `IF NOT EXISTS`, preservando a regra *append-only* das migrations SQL e evitando que `001_initial.sql` seja reexecutada em bancos existentes.

## Consequências

- **Vantagem:** Permite refatorações destrutivas e alterações de schema (`ALTER TABLE`) de forma segura, garantindo a execução de cada instrução *exactly-once*.
- **Vantagem:** Previne estados inconsistentes no banco de dados se uma migration falhar pela metade (tudo roda dentro de transações).
- **Desvantagem:** Requer cuidado ao escrever as migrations, pois uma migration já executada em produção não deve ser editada (append-only architecture para migrations).
