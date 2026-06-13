# ADR 011 — Subsistema de Resolução de Imagens via MTProto Proxy

## Status

Aceito

## Contexto

As URLs HTTP de mídia do Telegram (`cdn1.telesco.pe/file/...`, `cdn4.telegram-cdn.org/file/...`)
**não são persistentes** — expiram em ~48 horas retornando HTTP 404. Armazená-las no banco
é inútil para consumo futuro pelo frontend.

O RSShub resolveu esse problema na PR #13255 trocando de abordagem: ao invés de depender
das URLs HTTP efêmeras, passou a usar conexão client-level via MTProto para buscar a mídia
sob demanda e servi-la via proxy HTTP.

O Limiar já possui essa infraestrutura — `gotd/td` com sessão MTProto ativa no collector,
e o orquestrador `limiar run` (ADR 009) que roda todos os componentes no mesmo processo.

Os payloads brutos já coletados (`raw_messages.payload`) contêm os três campos MTProto
necessários para download sob demanda:

- `Photo.ID` — identificador permanente do arquivo (int64)
- `Photo.AccessHash` — hash de acesso (int64)
- `Photo.FileReference` — referência de sessão (base64, renovável)
- `Photo.DCID` — data center ID (int, necessário para routing)

## Decisão

### Abordagem: proxy MTProto sob demanda com cache in-memory

O frontend nunca acessa URLs do Telegram diretamente. O `limiar-api` expõe um endpoint
`GET /api/media/{msg_id}/photo` que faz stream dos bytes via MTProto. Um cache in-memory
com LRU (max 200 entradas) + TTL (30 min) absorve requests repetidos sem storage permanente.

### Onde vive: MediaResolver no orquestrador

O `MediaResolver` é um componente que vive dentro do orquestrador `limiar run`, compartilhando
o processo com o collector. Isso dá ao resolver acesso direto à conexão MTProto do collector
sem necessidade de IPC, gRPC, ou segunda sessão.

### Sem storage permanente de imagens

Imagens NÃO são salvas em disco. Motivação:
- Imagens podem ser re-baixadas via MTProto a qualquer momento (com renovação de `file_reference`)
- Elimina complexidade de gestão de arquivos, limpeza, e espaço em disco
- O cache in-memory com TTL é suficiente para a experiência de scroll infinito do frontend

### Singleflight para corretude em produção

Dois usuários pedindo a mesma imagem simultaneamente NÃO podem disparar dois `upload.GetFile`
MTProto independentes. `singleflight.Group` garante que a segunda request bloqueia e reutiliza
o resultado da primeira. Isso é **corretude**, não apenas otimização — evita flood no MTProto
e浪费 de banda.

### Renovação automática de file_reference em 3 níveis

| Nível | Trigger | Ação |
|-------|---------|------|
| L1 | Cache hit (LRU+TTL) | Retorna `[]byte` do cache. Zero chamadas MTProto |
| L2 | Cache miss | `upload.GetFile(photo_id, access_hash, file_reference, dcid)`. Se sucesso → cache → retorna. Se `FILE_REFERENCE_EXPIRED` → L3 |
| L3 | File reference expirado | **Soft:** `messages.getMessages(msg_id)` → atualiza `file_reference` no DB → retry. **Hard:** `FetchHistory(channel, msg_id, 1)` → atualiza todos os campos → retry |

### Processor: apenas metadados, sem download

O processor extrai e persiste os campos MTProto (`photo_access_hash`, `photo_file_ref`,
`photo_dcid`) mas NÃO baixa imagens. O download é responsabilidade exclusiva do MediaResolver.

## Schema

Novas colunas em `processed_messages` (migration 007):

```sql
ALTER TABLE processed_messages ADD COLUMN photo_access_hash INTEGER DEFAULT 0;
ALTER TABLE processed_messages ADD COLUMN photo_file_ref TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN photo_dcid INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_processed_media
    ON processed_messages(photo_id) WHERE photo_id > 0;
```

## Consequências

### Positivas
- Frontend com experiência completa: `<img src="/api/media/{id}/photo">` sempre funciona
- Zero gestão de arquivos em disco
- Cache bounded (10MB max com 200 entradas × 50KB)
- Renovação transparente para o usuário
- Singleflight previne flood MTProto

### Negativas
- Primeira request após cache miss tem latência MTProto (~200-500ms)
- Restart do processo esvazia o cache (cold start)
- Dependência da sessão MTProto ativa (se collector não está rodando, sem imagens)

### Riscos mitigados
- **Canal deletado**: L3 hard falha → imagem marcada como `unavailable` no log, endpoint retorna 404
- **Forward de outro canal**: `photo_id` + `access_hash` são globais, funcionam cross-canal
- **Imagem muito grande**: download limitado ao Size "x" (800px, ~50KB), não full resolution

## Alternativas consideradas

1. **Collector baixa tudo na coleta** — rejeitado: baixa fotos de mensagens inúteis (~10% de waste)
2. **Processor com acesso MTProto** — rejeitado: viola fronteira do processor (sem telegram)
3. **Cache em disco com TTL** — rejeitado: complexidade desnecessária, in-memory é suficiente
4. **URLs HTTP do Telegram** — rejeitado: expiram em 48h, inutilizável
