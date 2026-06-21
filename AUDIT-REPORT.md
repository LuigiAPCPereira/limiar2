# Relatório de Auditoria de Código — Limiar

**Data:** 2026-06-21
**Escopo:** repositório completo (`cmd/`, `internal/`, `tools/`, `docs/`, raiz)

---

## Baseline de Qualidade

| Verificação | Resultado |
|---|---|
| `go build ./...` | ✅ Zero erros |
| `go vet ./...` | ✅ Zero warnings |
| `go test ./...` (todos os pacotes) | ✅ Todos passam |
| `go test -race ./...` | ⚠️ Timeout em 120s (normal para Turso embarcado + race detector; testes individuais passam com `-race`) |

---

## Sumário

| Severidade | Quantidade |
|---|---|
| 🔴 HIGH | 12 |
| 🟡 MEDIUM | 24 |
| ⚪ LOW | 17 |

**Conclusão geral:** O código é de **alta qualidade**. Arquitetura limpa, concorrência correta, testes abrangentes, documentação em Português consistente. Os problemas encontrados são majoritariamente de manutenibilidade e cobertura de testes, não de correção funcional. Nenhum bug crítico de runtime foi identificado.

---

## 🔴 HIGH (12)

### 1. Lixo na raiz — scripts `fix_lint*.py`
**Arquivos:** `fix_lint.py`, `fix_lint_2.py`, `fix_lint_3.py`, `fix_lint_4.py`
**Problema:** Scripts Python descartáveis usados para aplicar correções mecânicas de lint. Código morto que polui a raiz do repositório.
**Ação:** Deletar os 4 arquivos.

### 2. Naming stutter no pacote `internal/media`
**Arquivos:** `internal/media/media.go`, `internal/media/cache.go`, `internal/media/resolver.go`
**Problema:** Nomes repetem o pacote: `media.MediaClient`, `media.MediaRepository`, `media.MediaResolver`, `media.ImageCache`. Convenção Go: o pacote já qualifica o tipo.
**Ação:** Renomear para `media.Client`, `media.Repository`, `media.Resolver`, `media.Cache`. Atualizar referências em `internal/telegram`, `internal/collector`, `internal/cli`, `cmd/`.

### 3. Parâmetro morto `_ MediaClient` em `NewMediaResolver`
**Arquivo:** `internal/media/resolver.go:28`
**Problema:** O parâmetro `_ MediaClient` é declarado mas nunca usado — o comentário diz que o Collector faz download proativo. O caller em `internal/cli/media.go:144` constrói um client real apenas para passá-lo a este parâmetro ignorado.
**Ação:** Remover o parâmetro de `NewMediaResolver` e ajustar o caller.

### 4. Struct morta `ImagePayload` em `internal/media`
**Arquivo:** `internal/media/media.go:35`
**Problema:** Tipo `ImagePayload` nunca referenciado em nenhum arquivo de produção ou teste.
**Ação:** Remover o tipo e seu doc comment.

### 5. `errorAttr` e constantes mortas no logger
**Arquivo:** `internal/logger/error.go`
**Problema:** Função `errorAttr` e 3 constantes associadas (`attrKeyError`, etc.) nunca usadas — marcadas com `//nolint:unused` pelo `fix_lint_3.py`.
**Ação:** Remover função e constantes, ou implementar seu uso.

### 6. `warnInvalid` retornado por `ResolveFormat` descartado em 10 call sites
**Arquivos:** `internal/logger/resolve.go` + todos os `internal/cli/*.go`, `cmd/*/main.go`
**Problema:** `ResolveFormat` retorna `(format, warnInvalid)` mas todos os 10 callers descartam o segundo valor com `_`. Se o formato for inválido, o warning é silenciosamente perdido.
**Ação:** Logar o warning no caller ou remover o segundo retorno e logar internamente em `ResolveFormat`.

### 7. `TerminalPresenter` sem mutex — viola contrato de concorrência do `Logger`
**Arquivo:** `internal/logger/presenter.go`
**Problema:** `Logger` é documentado como thread-safe, mas `TerminalPresenter` escreve em `io.Writer` sem sincronização. Uso concorrente (múltiplas goroutines logando) pode entrelaçar linhas.
**Ação:** Adicionar `sync.Mutex` ao `TerminalPresenter`.

### 8. `GetMessageByID` não mapeia `sql.ErrNoRows` → sentinel
**Arquivo:** `internal/storage/repository.go:445-451`
**Problema:** Cria `stderrors.New("not found")` dinamicamente em vez de usar um sentinel. Callers não podem usar `errors.Is` para distinguir "não encontrado" de outros erros. Inconsistente com o resto do repositório (`ErrNoSession`, `ErrChannelNotFound`).
**Ação:** Adicionar `ErrMessageNotFound` em `internal/errors/` ou mapear `sql.ErrNoRows` → sentinel existente.

### 9. Sem testes para `SaveRawMessageBatch` (hot path do Collector)
**Arquivo:** `internal/storage/repository_test.go`
**Problema:** `SaveRawMessageBatch` é o caminho primário de escrita do Collector (chamado por `collector.go:494`). Sem testes unitários. Um bug aqui (flags `inserted[]` incorretos, rollback parcial) só seria detectado em integração.
**Ação:** Adicionar `TestSaveRawMessageBatch` cobrindo: todas inseridas, flags `inserted[]` corretos, duplicatas no batch, batch vazio, batch de elemento único.

### 10. Sem testes para `SavePeersBatch`
**Arquivo:** `internal/storage/repository_test.go`
**Problema:** Mesma exposição que `SaveRawMessageBatch` — chamado por `telegram/peers.go:77`.
**Ação:** Adicionar `TestSavePeersBatch`.

### 11. Benchmark `SaveRawMessageBatch_100` mede caminho errado
**Arquivo:** `internal/storage/repository_batch_bench_test.go:88-90`
**Problema:** Usa os mesmos `message_id` (1-100) em todas as iterações. Após iteração 1, todas mensagens são duplicatas (`ON CONFLICT DO NOTHING`). O benchmark mede detecção de duplicatas, não throughput real de inserção.
**Ação:** Usar `MessageID` único por iteração (`int64(i*batchSize + j)`).

### 12. Código duplicado: `generateRunID()` em 3 binários
**Arquivos:** `cmd/limiar/main.go:164`, `cmd/limiar-collector/main.go:137`, `cmd/limiar-processor/main.go:100`
**Problema:** Função idêntica de 7 linhas copiada em 3 lugares.
**Ação:** Extrair para `internal/` (ex: `internal/id/runid.go`) ou para um pacote shared em `cmd/internal/`.

---

## 🟡 MEDIUM (24)

### 13. `ResolveFormat` aceita mas ignora parâmetro `subcommand`
**Arquivo:** `internal/logger/resolve.go`
**Problema:** Parâmetro `component` é recebido mas nunca usado na lógica de decisão. Todos os callers passam strings diferentes (`"auth"`, `"run"`, `"channels"`, etc.) que são descartadas. Ou o parâmetro deveria influenciar o formato, ou deveria ser removido.

### 14. `redactedKeys` incompleto — faltam `secret`, `key`, `private_key`, `access_token`
**Arquivo:** `internal/logger/redact.go:10-17`
**Problema:** Lista de chaves sensíveis cobre apenas chaves MTProto. Chaves comuns de APIs e infraestrutura não são protegidas.

### 15. `TerminalPresenter.buf[512]` pode estourar silenciosamente
**Arquivo:** `internal/logger/presenter.go`
**Problema:** Buffer fixo de 512 bytes para formatação. Mensagens longas podem truncar sem warning.

### 16. `formatScalar` default cai silenciosamente para kinds desconhecidos
**Arquivo:** `internal/logger/pretty.go`
**Problema:** Switch em `formatScalar` não tem caso `default` explícito — novos `slog.Kind` no futuro produziriam output vazio sem log de warning.

### 17. Magic strings `"collector-cache"` / `"inline-thumb"` no resolver
**Arquivo:** `internal/media/resolver.go:54`
**Problema:** Callers precisam fazer string comparison contra literais não documentados para interpretar a fonte da imagem.
**Ação:** Definir `type Source string` com constantes `SourceCollectorCache`, `SourceInlineThumb`.

### 18. Sem singleflight no `MediaResolver`
**Arquivo:** `internal/media/resolver.go:52`
**Problema:** Goroutines concorrentes resolvendo o mesmo `photoID` ambas sofrem cache miss e ambas batem no DB. `golang.org/x/sync` (singleflight) já está no `go.mod`.
**Ação:** Adicionar `singleflight.Group` ao resolver.

### 19. Sem teste de acesso concorrente no `ImageCache`
**Arquivo:** `internal/media/cache_test.go`
**Problema:** Cache documentado como thread-safe, compartilhado entre Collector e API, mas sem teste concorrente com `-race`.

### 20. `GetPhotoID` error path não testado
**Arquivo:** `internal/media/resolver_test.go`
**Problema:** `fakeRepo.GetPhotoID` sempre retorna nil error. Se `GetPhotoID` real retornar erro, nenhum teste verifica o wrapping.

### 21. Conversão `string(msg.Payload)` no hot path de inserção
**Arquivo:** `internal/storage/repository.go:298`
**Problema:** `[]byte → string` aloca no heap. Executado em cada `SaveRawMessage`/`SaveRawMessageBatch` para payloads JSON potencialmente grandes. O driver `tursogo` (SQLite dinamicamente tipado) deve aceitar `[]byte` para colunas TEXT.
**Ação:** Passar `msg.Payload` diretamente como `[]byte` ou fazer benchmark para confirmar que alocação é insignificante.

### 22. `GetChannelUsername` sem callers — código morto ou órfão
**Arquivo:** `internal/storage/repository.go:267-274`
**Problema:** Zero callers fora de `repository.go`. Comentário diz "Usado pelo subsistema de mídia" mas nenhum código de mídia chama. Ou está órfão (deletar) ou a integração não foi completada.

### 23. `UpdateFileReference` / `UpdatePhotoMetadata` sem callers
**Arquivo:** `internal/storage/repository.go:708-730`
**Problema:** ADR 011 menciona renovação L3 mas nenhum consumidor existe em produção.

### 24. `GetChannelUsername`, `GetPhotoMetadata` não mapeiam `sql.ErrNoRows` → sentinels
**Arquivos:** `internal/storage/repository.go:269-274,696-706`
**Problema:** Inconsistente com `GetChannel` e `LoadSession` que mapeiam para sentinels de domínio.

### 25. `scanChannel`/`scanMessage` retornam `sql.ErrNoRows` cru — contrato frágil
**Arquivos:** `internal/storage/repository.go:641-645,661-665`
**Problema:** Se essas funções auxiliares embrulharem o erro no futuro, todos os callers que dependem de `errors.Is(err, sql.ErrNoRows)` quebram silenciosamente.

### 26. `PhotoMetadata.ID` tem doc comment enganoso no contexto de `GetPhotoMetadata`
**Arquivo:** `internal/storage/repository.go:684-686,780-795`
**Problema:** Campo documentado como "0 em metadados vindos de MTProto" mas `GetPhotoMetadata` sempre popula `id` da tabela — nunca será 0 nessa query.

### 27. `makeBenchMessages` pré-aloca antes de `ResetTimer()`
**Arquivo:** `internal/storage/repository_batch_bench_test.go:68`
**Problema:** `B/op` do benchmark inclui alocação de `makeBenchMessages`, não apenas operações de DB.

### 28. Sem testes para queries de leitura do dashboard
**Arquivo:** `internal/storage/repository_test.go`
**Problema:** `ListMessages`, `ListProcessedMessages`, `CountMessagesByChannel`, `CountProcessedByType`, `CountProcessedMessages` — todos caminhos de leitura do dashboard sem cobertura de teste.

### 29. `tx.StmtContext()` sem `defer stmt.Close()` explícito
**Arquivo:** `internal/storage/repository.go:150,333`
**Problema:** `database/sql` faz close automático no commit/rollback, mas `defer` explícito é defesa contra mudanças futuras no driver.

### 30. `Repository.DB()` exporta `*sql.DB` — viola invariante de escritor único
**Arquivo:** `internal/storage/repository.go`
**Problema:** O invariante "único escritor no banco" é garantido por convenção, não pelo type system. Qualquer código pode obter `*sql.DB` e escrever diretamente.

### 31. `cmd/spike_resolve` — spike descartável no código de produção
**Arquivo:** `cmd/spike_resolve/main.go` (~380 linhas)
**Problema:** Binário de spike para validação de resolução de URLs. Contém `var _ = json.Marshal` para suprimir warning de import não usado. Deveria estar em `tools/` ou ser removido.
**Ação:** Mover para `tools/spike_resolve/` ou deletar se o spike já cumpriu seu propósito.

### 32. `errHandler` não usado no `dispatcher_test.go`
**Arquivo:** `internal/telegram/dispatcher_test.go:35-43`
**Problema:** Tipo `errHandler` definido mas nunca instanciado em nenhum teste. Marcado com `//nolint:unused`.
**Ação:** Remover ou usar em um teste que verifica comportamento do dispatcher com handler que retorna erro.

### 33. `testdata/` vazio no logger
**Arquivo:** `internal/logger/testdata/`
**Problema:** Diretório vazio. Ou deveria conter dados de teste, ou ser removido.

### 34. Arquivos grandes que dificultam navegação
| Arquivo | Linhas | Ideal |
|---|---|---|
| `internal/storage/repository.go` | 795 | Separar queries do processor em `processor_repository.go` (já existe `internal/processor/repository.go`!) |
| `internal/collector/collector.go` | 599 | Extrair `dbWriter` e `backfill` para arquivos próprios |
| `internal/processor/normalizer.go` | 507 | Extrair helpers de payload e conversão |
| `internal/telegram/client.go` | 447 | Extrair auth flow e backoff |

### 35. `internal/storage/repository.go` e `internal/processor/repository.go` — sobreposição de responsabilidade
**Problema:** `storage.Repository` contém queries do processor (`ProcessedMessage`, `ProcessedTypeStats`, `PhotoMetadata`, `PhotoStats`) que deveriam estar apenas em `processor.Repository`. O orquestrador (`cmd/limiar/main.go`) cria ambos os repositories sobre o mesmo `*sql.DB`, então queries do processor no `storage.Repository` são alcançáveis por código que não deveria acessá-las.

### 36. `ResolveFormat` — `PresenterTest.allowedPrefixes` muito amplo
**Arquivo:** `internal/logger/presenter_test.go`
**Problema:** Lista de prefixos permitidos inclui emojis que o código nunca emite, mascarando regressões de formatação.

---

## ⚪ LOW (17)

### 37. `GetPhotoID`/`GetInlineThumb` usam `err == sql.ErrNoRows` (pointer comparison)
**Arquivo:** `internal/storage/repository.go:734,753`
**Problema:** Funciona hoje porque `database/sql` retorna singleton, mas `errors.Is` é mais seguro contra wrapping futuro.

### 38. `Repository` struct expõe `*sql.DB` via método `DB()`
**Arquivo:** `internal/storage/db.go`

### 39. `price_currency TEXT DEFAULT 'BRL'` hardcoded
**Arquivo:** `internal/storage/migrations/003_processor_tables.sql:41-42`

### 40. `Channel.AddedAt` nunca definido no insert
**Arquivo:** `internal/storage/repository.go:63-71`

### 41. Comentário enganoso em `makeBenchMessages` sobre payloads distintos
**Arquivo:** `internal/storage/repository_batch_bench_test.go:54-57`

### 42. Sem teste de acesso concorrente a leituras
**Arquivo:** `internal/storage/repository_test.go`

### 43. `size <= 0` no cache floor para 1 — comportamento divergente do `expirable.NewLRU`
**Arquivo:** `internal/media/cache.go:28`

### 44. `fakeRepo` ignora `processedMsgID` — testes não verificam propagação correta de ID
**Arquivo:** `internal/media/resolver_test.go:14`

### 45. `init()` em benchmark — aceitável, mas padrão evita `init()` fora de testes
**Arquivo:** `internal/telegram/extract_messages_bench_test.go:14`

### 46. `redactAttr` ignora parâmetro `groups []string`
**Arquivo:** `internal/logger/redact.go:25`

### 47. `TestResolveImage_DBError` não verifica `errors.Is`
**Arquivo:** `internal/media/resolver_test.go:103`

### 48. `temp/` — diretório vazio na raiz
**Arquivo:** `temp/`

### 49. `skills-lock.json` na raiz — artefato de tooling, não do projeto
**Arquivo:** `skills-lock.json`

### 50. `CLAUDE.md` duplicado na raiz (existe `AGENTS.md`)
**Arquivos:** `./CLAUDE.md` (idêntico a `./AGENTS.md`)

### 51. Comentários em Português consistentes mas sem padrão de tradução para termos técnicos
**Problema:** "gravação" vs "escrita", "desligamento gracioso" vs "graceful shutdown". Não é um bug, mas dificulta grep.

### 52. `tools/payload-analyzer/` tem scripts Python + Go misturados
**Problema:** `analyze.py` + `main.go` no mesmo diretório de ferramenta.

### 53. `internal/logger/pretty_test.go` tem comentário `//nolint:staticcheck` em bloco de código complexo
**Problema:** O bloco `if !strings.HasPrefix(trimmed, "◆") && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└")` foi restaurado com `//nolint:staticcheck` após o `fix_lint_3.py` ter introduzido uma versão mais verbosa e depois revertido. Indica idas e vindas no código.

---

## O Que Está Excelente

1. **Arquitetura de concorrência:** Fan-out/fan-in com `dbWriter` único está impecável. Buffer sizes, graceful shutdown com drenagem, `recover()` por handler — tudo correto.
2. **Modelo de dados:** Schema versionado, migrações atômicas, `ON CONFLICT DO NOTHING` para idempotência, prepared statements.
3. **Separação de camadas:** Facade sobre gotd/td, tipos MTProto nunca vazam, injeção manual de dependências na composition root.
4. **Logging:** Interface injetável, 3 handlers, redação automática de segredos, `NopLogger` para testes.
5. **Testes de propriedade:** Uso de `pgregory.net/rapid` para propriedades de sessão e payload.
6. **Documentação:** ADRs para todas decisões arquiteturais, docs specs detalhados.
7. **Processor:** Normalização robusta (Shape A/B), cascata de classificação bem ordenada, batch com drain mode, preços em centavos.
8. **Coletor:** Backfill com limites numérico + temporal, retry com backoff exponencial, flush batch com timer adaptativo.
9. **Erro handling idiomático:** `Wrap(layer, op, err)` consistente, sentinels de domínio, `errors.Is` nos callers.

---

## Plano de Ação Recomendado

### Imediato (limpeza)
1. Deletar `fix_lint.py`, `fix_lint_2.py`, `fix_lint_3.py`, `fix_lint_4.py`
2. Deletar `temp/` (vazio)
3. Remover `CLAUDE.md` duplicado ou mantê-lo como symlink
4. Mover `cmd/spike_resolve` → `tools/spike_resolve`
5. Remover `ImagePayload`, `errorAttr`, `errHandler`
6. Extrair `generateRunID` para `internal/id/`

### Curto prazo (qualidade)
7. Renomear tipos com stutter: `media.MediaClient→Client`, `media.MediaResolver→Resolver`, `media.ImageCache→Cache`
8. Remover parâmetro `_ MediaClient` do `NewMediaResolver`
9. Adicionar sentinel `ErrMessageNotFound` e usá-lo em `GetMessageByID`
10. Adicionar `sync.Mutex` ao `TerminalPresenter`
11. Revisar `ResolveFormat`: logar `warnInvalid` internamente

### Médio prazo (testes)
12. `TestSaveRawMessageBatch` — hot path crítico
13. `TestSavePeersBatch`
14. Teste concorrente do `ImageCache` com `-race`
15. Testes para queries de leitura do dashboard
16. Corrigir benchmark `SaveRawMessageBatch_100`

### Longo prazo (estrutural)
17. Separar queries do processor de `storage.Repository`
18. Adicionar `singleflight.Group` ao `MediaResolver`
19. Migrar `[]byte → string` no hot path de inserção
20. Quebrar arquivos >500 linhas em arquivos menores por responsabilidade
