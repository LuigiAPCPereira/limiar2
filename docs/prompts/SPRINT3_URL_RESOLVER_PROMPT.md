# PROMPT — Sprint 3: URL Resolver

## Estado atual

Você está trabalhando no repositório `github.com/limiar/collector`.

Sprint 1 e Sprint 2 já foram implementados:

- Sprint 1: cupons múltiplos, modifiers estruturados, virtual_currency,
  Webpage.Title/URL/Description, schema e persistência.
- Sprint 2: Candidate Ranking Engine (CRE) determinístico para `product_name`,
  com `product_name_confidence`.

Medição real após calibração do CRE:

```text
deal_complete: 97.2% com product_name
deal_no_coupon: 99.3% com product_name
deal_no_price: 94.4% com product_name
```

Portanto, Sprint 3 NÃO é mais crítica para cobertura de ProductName. Ela é uma
sprint de enriquecimento/canonicalização de URL:

- canonicalizar URLs;
- remover tracking/affiliate params;
- cachear resoluções;
- extrair `<title>` como fonte complementar;
- preparar terreno para dedup por URL canônica e futura re-afiliação.

## Leitura obrigatória antes de alterar

Leia primeiro:

1. `AGENTS.md`
2. `docs/specs/EXTRACTION-CRE.md` — seção “Sprint 3 — URL Resolver”
3. `docs/adr/014-deterministic-extraction-and-cre.md`
4. `docs/adr/015-deployment-topology.md`
5. `docs/guidelines/STORAGE.md`
6. `docs/specs/TURSOGO.md`
7. `internal/processor/cre.go`
8. `internal/processor/cre_sources.go`
9. `internal/processor/normalizer.go`
10. `internal/storage/processor_repository.go`
11. `internal/storage/migrations.go`
12. `internal/storage/migrations/001_initial.sql`

Também consulte as skills aprovadas do projeto, se disponíveis:

- `turso-db`
- `cc-skills-golang`

Se elas não estiverem disponíveis, registre isso explicitamente antes de seguir.

## Premissas

1. Processor continua determinístico no core. URL Resolver adiciona rede, então
   precisa ser cacheado e falhar de forma graciosa.
2. Processor não importa `internal/telegram` nem `github.com/gotd/td`.
3. Sem LLM.
4. Sem nova dependência: usar `net/http`, `net/url`, `regexp`, `html`/stdlib.
5. Sem Playwright/Rod/browser automation nesta sprint.
6. Sem `products` e sem `price_history` nesta sprint.
7. Sem re-afiliação nesta sprint. Apenas limpar URL e persistir canonical URL.
8. Não implementar paralelismo/worker pool sem evidência. Comece simples.
9. Toda resolução de URL deve ter timeout curto e fallback.
10. Rede nunca pode travar o processamento inteiro.

## Objetivo da Sprint 3

Implementar URL Resolver como subsistema do processor para enriquecer mensagens
com dados de URL resolvida:

- `canonical_url`
- `url_title`
- `url_resolved`
- cache persistente `url_resolutions`

O resolver deve:

1. Receber uma URL original extraída de `NormalizedMessage`/`ProcessedMessage`.
2. Consultar cache persistente antes de fazer rede.
3. Se cache miss, fazer resolução HTTP com timeout.
4. Seguir redirects com limite.
5. Remover tracking params e affiliate params.
6. Extrair `<title>` da resposta HTML quando disponível.
7. Persistir resolução no cache.
8. Persistir resultado na mensagem processada.
9. Falhar de forma graciosa: se timeout/403/erro, manter URL original e marcar
   `url_resolved = 0`.

## Design alvo

Pipeline esperado:

```text
Normalize(raw)
    ↓
Sprint 1 extraction
    ↓
CRE (WebpageTitle + TextHeuristic)
    ↓
URL Resolver (cache-first, network fallback)
    ↓
Se product_name vazio e url_title existe:
    reexecutar CRE com URLTitleSource/cache OU preencher apenas se score >= threshold
    ↓
SaveProcessed/SaveProcessedBatch
```

Observação importante: como a cobertura atual do CRE já é alta, não arrisque
regredir o ProductName. URLTitleSource é complementar e deve ser usado com
cuidado:

- WebpageTitle continua com prioridade alta.
- TextHeuristic continua funcionando sem rede.
- URLTitleSource só deve ajudar quando `product_name == ""` ou quando o título
  resolvido tem score claramente maior que o candidato atual.

Se houver ambiguidade, prefira NÃO reexecutar o CRE no primeiro PR e apenas
persistir `url_title`/`canonical_url`. A integração do URLTitleSource pode ser
um segundo PR menor.

## Estrutura de código sugerida

```text
internal/processor/
├── urlresolver.go              # Resolver, URLResolution, ResolveURL
├── urlresolver_test.go
├── urlresolver_canonical.go    # limpeza/canonicalização pura
├── urlresolver_title.go        # extração de <title>
├── cre_sources.go              # URLTitleSource, se integrado nesta sprint
└── normalizer.go               # chamada ao resolver, se a arquitetura atual permitir

internal/model/
├── normalized.go               # novos campos se necessário
└── processed.go                # CanonicalURL, URLTitle, URLResolved

internal/storage/
├── processor_repository.go     # Save/Get URLResolution + persistência dos campos
├── migrations.go               # currentSchemaColumns + table definitions se necessário
└── migrations/
    └── 010_url_resolver.sql    # marcador + CREATE TABLE idempotente se aplicável
```

Evite criar interfaces abstratas no primeiro passo. Uma struct concreta `URLResolver`
com `*http.Client`, repository e logger é suficiente.

## Schema e migration strategy

Siga a regra documentada em `docs/guidelines/STORAGE.md`:

- `001_initial.sql`: schema consolidado para bancos novos.
- `migrations.go/currentSchemaColumns`: adiciona colunas ausentes em bancos
  legados via `PRAGMA table_info`.
- Arquivo `010_url_resolver.sql`: marcador idempotente e/ou criação de tabela nova
  idempotente.
- NÃO colocar `ALTER TABLE ADD COLUMN` direto no arquivo numerado.

### Tabela nova

Criar tabela:

```sql
CREATE TABLE IF NOT EXISTS url_resolutions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    original_url  TEXT NOT NULL UNIQUE,
    canonical_url TEXT NOT NULL DEFAULT '',
    merchant      TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    unresolved    INTEGER NOT NULL DEFAULT 0,
    resolved_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_url_resolutions_canonical
    ON url_resolutions(canonical_url) WHERE canonical_url != '';
```

Também incluir essa tabela em `001_initial.sql` para bancos novos.

### Colunas em processed_messages

Adicionar a `processed_messages`:

```go
canonical_url TEXT NOT NULL DEFAULT ''
url_title     TEXT NOT NULL DEFAULT ''
url_resolved  INTEGER NOT NULL DEFAULT 0
```

Essas colunas entram em:

- `001_initial.sql`
- `currentSchemaColumns`
- model `ProcessedMessage`
- SaveProcessed/SaveProcessedBatch
- ListProcessedMessages
- testes de round-trip

## Canonicalização de URL

Implementar função pura, testável:

```go
func CanonicalizeURL(raw string) (string, error)
```

Regras mínimas:

1. Parse com `net/url`.
2. Lowercase em scheme e host.
3. Remover fragment (`#...`).
4. Remover tracking params conhecidos:
   - `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`
   - `fbclid`, `gclid`, `msclkid`
   - `ref`, `ref_`, `referrer`
   - `tag`, `ascsubtag`, `linkCode`, `creative`, `camp`
   - `affiliate_id`, `aff_id`, `mmp_pid`, `mmp_sub1`, `mmp_sub2`
   - `spm`, `algo_pvid`, `algo_exp_id`
5. Ordenar query params restantes para estabilidade determinística.
6. Preservar path e query útil.
7. Não tentar “adivinhar” produto por regex frágil de merchant.

Testar com Amazon, Shopee, Mercado Livre, Magalu e AliExpress.

## Resolução HTTP

Implementar cache-first:

```text
Resolve(originalURL):
  if originalURL == "": return empty
  if cache has originalURL: return cache
  ctx timeout 5s
  HTTP GET/HEAD com follow redirects max 10
  canonicalize final URL
  if HTML: extract <title>
  save cache
  return result
```

Preferência:

- Começar com GET limitado a um tamanho máximo de leitura (ex: 256KB) para
  conseguir extrair `<title>` sem baixar página inteira.
- HEAD pode falhar em alguns merchants; GET limitado é mais confiável.
- `http.Client{Timeout: 5 * time.Second}`.
- `CheckRedirect`: max 10 redirects.
- User-Agent de browser realista.

Não fazer:

- retries agressivos;
- crawl paralelo;
- browser automation;
- bypass anti-bot;
- resolver URL se cache já existe.

## Extração de `<title>`

Função pura:

```go
func ExtractHTMLTitle(html []byte) string
```

Regras:

- Regex case-insensitive para `<title ...>...</title>`.
- Decodificar entidades HTML básicas com stdlib (`html.UnescapeString`).
- Collapse whitespace.
- Limitar tamanho máximo do título (ex: 200 chars).
- Retornar string vazia se não encontrar.

## Integração com CRE

Opção segura para primeiro PR:

1. Persistir `url_title`.
2. Persistir `canonical_url`.
3. Persistir `url_resolved`.
4. NÃO alterar decisão do `product_name` ainda, exceto se o spec/testes exigirem.

Opção completa, se implementar com segurança:

- Adicionar URLTitleSource em `cre_sources.go`.
- Rodar apenas se `product_name == ""` e `url_title != ""`.
- Usar base confidence menor que WebpageTitle, ex: 0.55.
- Manter threshold 0.45.
- Testar que casos bons melhoram e que casos já bons não regrediram.

Como a cobertura atual já é alta, priorize não-regressão.

## Repository

Adicionar métodos concretos no repository, sem interface prematura:

```go
SaveURLResolution(ctx context.Context, r URLResolution) error
GetURLResolution(ctx context.Context, originalURL string) (URLResolution, bool, error)
```

Se os tipos em `internal/processor` gerarem ciclo de import, mova `URLResolution`
para `internal/model` ou defina um DTO no storage. Não crie dependência circular.

## Testes obrigatórios

### Canonicalização

- remove UTM params;
- remove `fbclid/gclid`;
- remove Amazon affiliate `tag`;
- remove Shopee/Ali tracking params;
- preserva params úteis;
- ordena query params;
- lowercase host;
- remove fragment;
- URL inválida retorna erro.

### Title extraction

- `<title>Produto X - Loja</title>`;
- title com whitespace/newlines;
- title com entidades HTML;
- HTML sem title;
- title muito longo é truncado.

### Resolver HTTP

Use `httptest.Server`:

- redirect 301/302 até URL final;
- max redirects;
- timeout;
- 403 vira unresolved;
- HTML com title;
- cache hit evita segunda chamada HTTP.

### Repository/schema

- migration cria `url_resolutions`;
- round-trip de URLResolution;
- campos `canonical_url`, `url_title`, `url_resolved` persistem em
  processed_messages;
- banco legado sem colunas é reparado por `currentSchemaColumns`.

### Não regressão CRE

- reprocessar casos já testados de CRE;
- URLTitleSource não deve sobrescrever WebpageTitle bom;
- URLTitleSource só preenche se `product_name` vazio ou se claramente superior.

## Verificação obrigatória

```sh
go build ./...
go vet ./...
go test ./internal/processor/... ./internal/storage/...
go test ./...
go test -race ./...
```

Depois, se o resolver estiver integrado ao reprocessamento:

```sh
limiar processor reprocess --all
```

Medir e reportar:

- `% processed_messages com canonical_url != ''`
- `% processed_messages com url_title != ''`
- `% processed_messages com url_resolved = 1`
- cache hit/miss se instrumentado
- tempo total de reprocessamento vs baseline atual (~2m)
- número de falhas/timeout/403

Se rede real tornar o reprocessamento muito lento, pare e documente. Não deixe
uma implementação que faça 31k requests reais sem cache/limite.

## Guardrails de performance

O resolver é o primeiro estágio com rede. Cuidado:

- Não resolver URL em hot path sem cache.
- Não resolver múltiplas vezes a mesma URL.
- Não fazer mais de uma request por URL original em cache miss.
- Não bloquear batch inteiro por uma URL lenta.
- Timeout máximo 5s por URL.
- Para reprocessamento histórico, considerar flag para desabilitar rede ou limitar
  quantidade resolvida por execução.

Sugestão de segurança:

```text
--resolve-urls=false por padrão no reprocess histórico
--resolve-urls-limit N para backfill controlado
```

Se essa flag ainda não existir, avalie adicionar no comando de reprocess, mas não
mude o comportamento padrão sem documentar.

## O que NÃO fazer

- Não implementar `products`.
- Não implementar `price_history`.
- Não implementar re-afiliação.
- Não adicionar Playwright/Rod/chromedp.
- Não adicionar dependência HTTP externa.
- Não remover `raw_messages`.
- Não transformar o processor em crawler.
- Não fazer worker pool por reflexo.
- Não quebrar `go test -race ./...`.

## Entregável esperado

1. URL resolver implementado com cache persistente.
2. Schema/migrations compatíveis com a estratégia do projeto.
3. Testes cobrindo canonicalização, title extraction, resolver HTTP e repository.
4. Integração segura com processed_messages.
5. Relatório final com verificação real e, se executado, métricas de resolução.

## Observação estratégica

Como o CRE já atingiu cobertura alta, a Sprint 3 deve ser tratada como sprint de
qualidade de URL, dedup futuro e preparação para produto/API — não como correção
urgente de ProductName.

A implementação mínima aceitável é:

- cache `url_resolutions`;
- `canonical_url`;
- `url_title`;
- `url_resolved`;
- testes robustos;
- sem regressão de performance.
