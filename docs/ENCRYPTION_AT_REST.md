# Encryption at Rest — Referência Futura

> [!NOTE]
> Este arquivo documenta a possibilidade de encryption at rest para o `limiar.db`.
> A funcionalidade NÃO foi implementada. Serve como memória caso seja retomada no futuro.

## O que é

Encryption at rest protege os dados no arquivo `.db` contra acesso não-autorizado ao filesystem.
O principal ativo sensível é a **sessão MTProto** (credencial equivalente à conta Telegram).

## Como funciona no Turso

O Turso suporta encryption at rest via URI format:

```
file:limiar.db?cipher=aes256gcm&hexkey=<key_hex_64_chars>
```

### Requisitos

1. **Flag experimental**: `--experimental-encryption` (Turso v0.6.x)
2. **Re-criação do banco**: não é possível criptografar um banco existente in-place
3. **Chave externa**: 32 bytes hex (64 chars), gerenciada via env var ou vault
4. **Performance**: encryption por página — overhead desconhecido para Turso

### Implementação proposta

```go
// Em storage.Open(), construir DSN com encryption:
dsn := dbPath
if encKey := os.Getenv("LIMIAR_DB_ENCRYPTION_KEY"); encKey != "" {
    dsn = fmt.Sprintf("file:%s?cipher=aes256gcm&hexkey=%s", dbPath, encKey)
}
conn, err := sql.Open(driverName, dsn)
```

### Migração de banco existente

1. Abrir banco antigo (sem encryption)
2. `.dump` via SQL ou export programático
3. Criar banco novo com encryption
4. Importar dados
5. Substituir arquivo

### Modelo de ameaça

| Cenário | Protege? |
|---------|----------|
| Arquivo `.db` copiado do filesystem | ✅ Sim |
| Acesso à máquina com leitura de env vars | ❌ Não (chave em env) |
| Acesso ao processo em runtime (memory dump) | ❌ Não |
| Backup não-criptografado do `.db` | ✅ Sim |

### Variáveis de configuração necessárias

| Variável | Descrição |
|----------|-----------|
| `LIMIAR_DB_ENCRYPTION_KEY` | Chave hex de 64 chars (32 bytes) |
| `LIMIAR_DB_ENCRYPTION_ENABLED` | `true`/`false` |

### Referências

- [Turso encryption docs](https://docs.turso.tech) (verificar versão atual)
- Skill `turso-db` — seção sobre encryption
- ADR necessário antes de implementar (AGENTS.md §19)

### Decisão

**2026-06-11:** Owner decidiu não implementar nesta fase. Razão: modelo de ameaça
não justifica a complexidade. `chmod 0600` + processo local já cobrem o cenário
principal. Encryption at rest é uma camada adicional, não uma muralha.
