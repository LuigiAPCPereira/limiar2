# Limiar 3.0 — herança comprovada da Rebaseline 2026

**Estado:** documentação derivada para orientar a reconstrução Limiar 3.0. Não cria Decision, não aceita ADR Proposed e não autoriza implementação por si só.

## Regra principal de herança

> **Limiar 3 herda as conclusões, invariantes, contratos aceitos e limites de Evidence da rebaseline; não herda automaticamente mecanismos experimentais, schemas Proposed, topologia física, packages ou decisões acidentais do Limiar legado.**

Quando este documento resumir uma fonte, a autoridade continua na fonte original conforme `AGENTS.md`.

## 1. O que é autoridade e o que não é

A rebaseline consolidou a separação epistemológica entre **Evidence**, **Finding**, **Hypothesis**, **Experiment**, **Proposal** e **Decision**. Resultado de experimento `Supported` não equivale a ADR `Accepted`; código, schema e documentação derivada também não criam autoridade por existência.

Para o Limiar 3, reutilizar uma implementação exige confirmar que ela satisfaz um contrato vigente. Um experimento verde nunca vira default de produção por inércia.

## 2. Fundação aceita que deve sobreviver

### 2.1 Evidence e sincronização — ADR 016

Preservar:

- Source Evidence append-only/versionada;
- identidade física de Evidence separada da identidade lógica da mensagem;
- replay permitido sem promessa de exactly-once;
- live sync baseado no state nativo do Telegram, não em `LastMessageID`;
- backfill com lifecycle/progresso próprio;
- progresso nunca certificando durabilidade que ainda não existe.

Estado corrente, feed e entidades comerciais são derivados reconstruíveis, não substitutos da Evidence admitida.

### 2.2 Recovery — ADR 017

O gotd continua responsável pelo ordering/recovery especializado do Telegram. O Limiar mantém boundaries explícitos ao redor dele.

```text
Evidence exigida durável
        ↓
state/progress pode avançar
```

Uma durability barrier fechada é terminal para aquela instância. Erro de leitura não vira ausência. Replay após restart é esperado. `differenceTooLong` e resets precisam continuar observáveis; o sistema não pode afirmar que observou eventos irrecuperáveis.

### 2.3 Source Admission — ADR 018

O envelope live relevante deve atravessar Source Admission durável antes do `updates.Manager`.

```text
Telegram transport
  -> Source Admission
  -> Evidence
  -> updates.Manager/recovery
  -> trabalho derivado
```

O handler pós-Manager não é autoridade do envelope bruto original. Trabalho derivado reconstruível pode falhar sem invalidar Source Evidence já durável.

### 2.4 SQLite e Evidence — ADRs 019 e 020

A base aceita do novo banco é SQLite via `github.com/ncruces/go-sqlite3`, `database/sql`, migrations versionadas, capabilities estreitas e baseline operacional conservadora.

O schema mínimo aceito de Evidence preserva:

- UUIDv4 criptograficamente aleatório em `BLOB(16)`;
- `subscription_id` obrigatório;
- payload byte-preserving;
- `payload_format`/`payload_schema` explícitos;
- SHA-256 calculado no boundary de persistência;
- timestamps independentes da identidade;
- duplicidade/replay não colapsados por hash ou message ID;
- tabela `STRICT`;
- capability de append e guards contra `UPDATE`/`DELETE`;
- nenhum índice de domínio sem workload demonstrado.

O novo SQLite nasce **side-by-side**. Tursogo não deve sofrer migração in-place e permanece histórico/read-only para compatibilidade, auditoria, importação e rollback enquanto necessário.

## 3. O que continua Proposed

### ADR 021 — SourceSyncState físico

Permanece `Proposed`. Há Evidence para common state completo, channel state separado, ausência distinta de zero/erro e setters parciais sem fabricar state, mas o schema não está autorizado para produção.

### ADR 022 — BackfillProgress físico

Permanece `Proposed`. Há Evidence para progresso próprio por subscription, avanço monotônico e Evidence-before-progress, mas a materialização física depende de aceitação.

### ADR 023 — sessão MTProto hardened

Permanece `Proposed`. Limiar 3 ainda não escolheu arquivo como session storage.

### ADR 024 — Acquisition Subscription Identity

Permanece `Proposed`. ADR 020 exige `subscription_id` em Evidence, mas o runtime não pode inventá-lo a partir de `channel_id`. A authority/configuração da subscription precisa ser resolvida antes do Source Admission produtivo.

## 4. Sessão MTProto e peer cache são authorities diferentes

F-STO-006 demonstrou que a co-localização legada de `sessions` e `peers` não justifica uma authority futura única.

**Sessão** é credencial sensível e durável. Precisa tratar ausência versus erro, overwrite seguro, proteção do segredo, restart, coordenação entre clients, recovery/backup conscientes de segredo e suporte de plataforma comprovado.

**Peer cache** é estado operacional para `access_hash` e resolução Telegram. Não é subscription truth, não é session authority e não deve virar source of truth apenas porque existe uma tabela legada.

## 5. O que os experimentos de sessão provaram

### EXP-LIMIAR-018 — Rejected

`gotd/session.FileStorage v0.161.0` **as-is** foi rejeitado como boundary final no ambiente observado. O contrato funcional básico funcionou, mas arquivo preexistente permissivo não foi hardened e a implementação usa `os.WriteFile`, cujo contrato admite conteúdo parcial em falha intermediária.

Isso não rejeita estratégia baseada em arquivo; rejeita usar o upstream sem adaptação como boundary final.

### EXP-LIMIAR-019 — Supported com escopo estreito

Um harness experimental mostrou, no ambiente Unix/Linux e intra-processo observado, que uma camada pequena pode:

- substituir arquivo preexistente `0644` por destino `0600`;
- escrever em temporário no mesmo diretório;
- sincronizar conteúdo antes da publicação;
- publicar por rename;
- sincronizar diretório quando aplicável;
- serializar writers do mesmo processo por path;
- terminar sem temporários remanescentes e sem race detectada no gate executado.

O resultado **não prova** ACL/replace Windows, coordenação multiprocesso, power-loss, criptografia em repouso, integração real com gotd ou política de backup/restore. Também não escolhe arquivo sobre alternativas.

## 6. Portabilidade: Evidence com limites

A rebaseline acumulou Evidence para backup/restore; locking/WAL em Linux X64; `CGO_ENABLED=0` em Linux X64; cross-build para Linux ARM64, Windows AMD64 e Darwin ARM64; execução nativa de `internal/storage/sqlite` em Linux ARM64 e Windows AMD64 no ambiente observado.

Limites:

- não equivale a suporte completo do produto;
- não prova Windows ARM64 nem UNC;
- não prova ACL Windows equivalente a `0600`;
- EXP-LIMIAR-016 ficou inconclusivo em macOS por ausência de worker executável.

Para drive letter no SQLite Windows foi necessário tratamento equivalente a `file:C:/...`; semântica POSIX de path/permissão não deve ser presumida no Windows.

## 7. Migração histórica ainda não está provada

EXP-LIMIAR-007 foi alinhado ao storage atual e abre o destino por `internal/storage/sqlite.Open`, usando migrations/baseline vigentes. O gate continua aberto porque nenhuma cópia histórica descartável real do banco legado foi exercitada.

Limiar 3 deve:

1. construir authorities novas side-by-side;
2. comparar contra o legado;
3. exercitar importação sobre cópia histórica real descartável;
4. manter rollback;
5. só então considerar desativação/remoção do legado.

## 8. Matriz de herança

| Área | Herança | Tratamento no Limiar 3 |
| --- | --- | --- |
| Evidence | ADR 016/018/019/020 | Reutilizar contratos e revalidar integração |
| SQLite | engine/baseline/migrations/capabilities aceitas | Reaproveitar onde o contrato for o mesmo |
| Live recovery | ADR 016/017/018 | Reconstruir wiring limpo em torno do gotd |
| SourceSyncState | ADR 021 Proposed | Não materializar sem aceite |
| BackfillProgress | ADR 022 Proposed | Não materializar sem aceite |
| Session storage | F-STO-006 + EXP-018/019; ADR 023 Proposed | Investigar/decidir antes do runtime |
| Subscription identity | lacuna real; ADR 024 Proposed | Resolver antes de Source Admission produtiva |
| Peer state | cache operacional separado | Definir reconstrução/lifecycle; não copiar tabela |
| Tursogo | legado histórico/read-only | Compatibilidade/importação/rollback |
| Processamento | direção conceitual da baseline | Construir como derivados reprocessáveis |
| MCP/API/frontend | escopo L3 | Consumir Query Service; nunca source of truth |

## 9. Consequência para a construção

A fundação deve ser construída por **authorities e contratos**, não pela árvore de packages do legado:

```text
invariantes/config/segredos
  -> sessão MTProto
  -> storage de Evidence
  -> subscription + Source Admission
  -> recovery/live sync
  -> backfill
  -> peer/media
  -> projeções reconstruíveis
  -> interpretação comercial
  -> Product/Listing/Offer/Relations
  -> Query Service
  -> MCP/API
  -> frontend
  -> migração/cutover
```

O plano operacional detalhado está em [`BOTTOM_UP_REBUILD_PLAN.md`](BOTTOM_UP_REBUILD_PLAN.md).
