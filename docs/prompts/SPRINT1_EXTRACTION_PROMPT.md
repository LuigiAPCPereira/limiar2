# PROMPT — Sprint 1: Correções de Extração + Webpage + Schema

## O QUE ESTÁ ACONTECENDO

Você está atuando como engenheiro sênior no projeto **Limiar** (`github.com/limiar/collector`), um pipeline Go que captura promoções de canais do Telegram e as processa em dados estruturados. Este é o **Sprint 1 de 3** de uma sequência determinística de melhorias de extração.

**Este prompt foca APENAS no Sprint 1.** Não implemente o Sprint 2 (CRE) nem o Sprint 3 (URL Resolver).

---

## LEITURA OBRIGATÓRIA ANTES DE CODAR

Você tem acesso ao repo. Leia estes arquivos antes de tocar em qualquer código. Eles são a fonte da verdade e vencem qualquer palpite:

1. **`AGENTS.md`** — Regras obrigatórias, invariantes arquiteturais e stack fechada. Aprenda a hierarquia de autoridade (§1).
2. **`docs/specs/EXTRACTION-CRE.md`** — A especificação COMPLETA deste Sprint 1. Siga a seção "Sprint 1" rigorosamente.
3. **`docs/adr/014-deterministic-extraction-and-cre.md`** — Contexto arquitetural do porquê estamos fazendo isso.
4. **`docs/guidelines/STORAGE.md`** — Regras de migrations e SQL (placeholders `?`, prepared statements).
5. **`internal/processor/normalizer.go`** — O arquivo central que você vai modificar.
6. **`internal/model/normalized.go`** e **`internal/model/processed.go`** — Onde vivem as structs de domínio.
7. **`internal/storage/migrations.go`** e **`internal/storage/migrations/001_initial.sql`** — Sistema de migrations.

---

## O PROBLEMA (Resumo)

Uma análise do banco de produção (31.832 mensagens) revelou três categorias de sub-extração e uma funcionalidade ausente:

1. **Cupons múltiplos perdidos**: A função `extractCoupon` usa `FindStringSubmatch` (first-only), ignorando padrões " ou " e " + " documentados no spec.
2. **Modifiers sub-extraídos**: PIX perde 24% das menções (`(pix)`, `SELECIONE PIX`). VirtualCurrency (Moedas Shopee/AliExpress), App-only e Web-only têm 100% de loss.
3. **Webpage.Title não persistido**: O normalizer extrai `Webpage.URL` mas ignora `Title` e `Description`, que são fontes gold standard para futuras extrações.
4. **Schema desatualizado**: Falta persistir dados estruturados como JSON (cupons múltiplos, modifiers, virtual_currency, webpage fields).

---

## ESCOPO DA TAREFA (Sprint 1)

Implementar a extração determinística melhorada conforme a seção "Sprint 1" de `docs/specs/EXTRACTION-CRE.md`. Especificamente:

### 1. Cupons Múltiplos
- Criar `internal/processor/extract_coupons.go`.
- Função: `extractCoupons(text string) []model.Coupon`.
- Struct `Coupon` em `internal/model/normalized.go`: `{Code, DiscountType, DiscountValue, RequiresAction}`.
- Usar `FindAllStringSubmatch` (ALL matches), fazer split por " ou " e " + ".
- Detectar cupom de valor (`fixed_brl`, `percent`) e requires_action (`resgate no anúncio`).
- Dedup por código case-insensitive.
- **Compatibilidade**: o campo `coupon_code` existente continua sendo o primeiro código extraído (para não quebrar o dashboard/queries).

### 2. Modifiers Estruturados
- Criar `internal/processor/extract_modifiers.go`.
- Função: `extractModifiers(text string) []model.Modifier`.
- Struct `Modifier` em `internal/model/normalized.go`: `{Type, Value}`.
- Types: `"payment"`, `"shipping"`, `"installments"`, `"cashback"`, `"discount"`, `"app_only"`, `"web_only"`, `"recurring"`.
- **Refinar regex do PIX** para cobrir os 24% perdidos: `(pix)`, `SELECIONE PIX NA PAG`, `pix na pag`.
- **Adicionar detecção**: `app_only`, `web_only`, e `VirtualCurrency` (Moedas).
- Manter os campos flat existentes (`payment_method`, `shipping`, etc.) — o campo JSON `modifiers` é ADICIONAL, não substitui.

### 3. VirtualCurrency
- Struct `VirtualCurrency` em `internal/model/normalized.go`: `{Platform, Amount, CapBRL, Type}`.
- Regex para Moedas (ex: `1853 MOEDAS`), conforme spec §1.3.
- Plataforma inferida do merchant (`shopee` ou `aliexpress`).

### 4. Webpage.Title/URL/Desc
- Criar `internal/processor/extract_webpage.go`.
- Função: `extractWebpageInfo(msg map[string]any) WebpageInfo`.
- Struct `WebpageInfo` em `internal/model/normalized.go`: `{URL, Title, Description}`.
- Extrair de `Media.Webpage.{URL,Title,Description}` no payload bruto.
- Persistir SEMPRE que presente, independente de o texto ter URL.

### 5. Schema Migration
- Criar `internal/storage/migrations/008_extraction_enhancements.sql` com o SQL exato da spec §1.5.
- Registrar as 7 colunas novas no slice `currentSchemaColumns` em `internal/storage/migrations.go`.
- Atualizar `internal/storage/migrations/001_initial.sql` com as mesmas colunas (para que bancos novos nasçam completos).

### 6. Atualização do Model e Repository
- Adicionar campos a `model.NormalizedMessage` e `model.ProcessedMessage` (spec §1.6).
- Atualizar `storage.ProcessorRepository` (Save/Scan) para ler/escrever as novas colunas.
- Serializar `coupon_codes`, `modifiers`, `virtual_currency` como JSON TEXT.

---

## REGRAS RÍGIDAS (AGENTS.md — não negociáveis)

- **Stack Fechada (§11)**: Apenas as dependências listadas. NENHUMA dependência nova.
- **Preços são INTEGER (centavos)**: Nunca `float64` para valor monetário (§15.1).
- **Processor não importa Telegram**: `internal/processor` nunca importa `internal/telegram` (§15.1).
- **Todo SQL em `internal/storage`**: Placeholders são apenas `?` (§13.7).
- **Sem `init()` / estado global mutável** em `internal/` (§12.3).
- **`context.Context` primeiro argumento** de toda I/O (§12.1).
- **Erros com wrapping**: Formato `"layer: op: cause"` via `apperrors.Wrap` (§12.2).

---

## PRINCÍPIOS DE ENGENHARIA (do Spec)

1. **Alta coesão, baixa acoplamento**: Cada extractor é uma função pura (texto → struct). Não conhece banco, não conhece telegram.
2. **Extensível sem refatoração**: Novos padrões de regex são adicionados como dados (slices de struct), não como novos `if`.
3. **Limpo e legível**: Nomes descritivos, funções curtas, comentários explicando "porquê". Zero magic numbers — constantes nomeadas.
4. **Testável**: Tabela de testes (`[]struct{ in, want }`) com casos reais do corpus. Teste de regressão para cada bug corrigido.
5. **Determinístico**: Mesmo input → mesmo output. Sem `time.Now()` em extração.
6. **Sem abstração prematura**: Funções concretas. Não criar interfaces sem 2+ implementações reais.

---

## CASOS DE TESTE OBRIGATÓRIOS

Use casos REAIS do corpus (presentes no spec e no relatório de análise). Cada arquivo de teste deve cobrir:

### `extract_coupons_test.go`
- **Simples**: `cupom: TOCAPROGOL` → `[{"code":"TOCAPROGOL"}]`
- **Múltiplo " ou "**: `Cupom: AEBR2 ou IFPL90V1` → 2 códigos
- **Múltiplo " + "**: `Cupom: NOTE600 + POUPE` → 2 códigos
- **Valor fixo**: `Cupom de R$ 30 OFF` → `fixed_brl`, value 3000
- **Percentual**: `Cupom de 15% OFF` → `percent`, value 15
- **Requires_action**: `Resgate o cupom no anúncio` → `requires_action: true`
- **Vazio**: Texto sem menção → `[]`

### `extract_modifiers_test.go`
- **PIX variantes**: `POR: 99 (pix)`, `SELECIONE PIX NA PAG`, `no pix`
- **App-only**: `APENAS PELO APLICATIVO`, `somente no app`
- **Web-only**: `pela web`, `somente no site`
- **Moedas**: `1853 MOEDAS`, `50% cashback em Moedas Shopee`
- **Combinações**: pix + frete grátis + parcelamento na mesma mensagem

### `extract_webpage_test.go`
- **Completo**: Payload com Webpage (URL+Title+Desc) → extrai os 3
- **Sem Webpage**: Payload com Photo (Media null) → retorna vazio
- **Parcial**: Payload com Webpage mas Title vazio → URL preenchido, Title vazio

---

## ESTRUTURA DE ARQUIVOS ESPERADA

```
internal/model/
├── normalized.go          (MODIFICADO)
└── processed.go           (MODIFICADO)

internal/processor/
├── normalizer.go          (MODIFICADO — orquestra novos extractors)
├── extract_coupons.go     (NOVO)
├── extract_modifiers.go   (NOVO)
├── extract_webpage.go     (NOVO)
├── extract_coupons_test.go
├── extract_modifiers_test.go
└── extract_webpage_test.go

internal/storage/
├── migrations/008_extraction_enhancements.sql  (NOVO)
├── migrations/001_initial.sql                  (MODIFICADO)
├── migrations.go                               (MODIFICADO)
└── processor_repository.go                     (MODIFICADO)
```

---

## O QUE NÃO FAZER

- **Não implemente o Sprint 2 (CRE) ou Sprint 3 (URL Resolver).** Só o Sprint 1.
- **Não mude a concorrência do processor** (continua single-threaded, poll loop).
- **Não adicione dependências externas.**
- **Não crie abstrações/interfaces prematuras.** Funções puras.
- **Não faça refactors drive-by** em código estável que não foi pedido.
- **Não invente campos, APIs ou imports** que não existem no repositório.

---

## VERIFICAÇÃO (deve passar antes de considerar pronto)

```sh
go build ./...           # saída 0, zero erros
go vet ./...             # zero problemas
go test ./...            # todos passam
go test -race ./...      # sem data races
```

O build NÃO deve conter nenhum import de `sqlite`, `mattn`, `gorm` ou qualquer lib fora da closed stack.

**Verificação de schema (idempotência):**
- Abrir um banco NOVO (vazio) → migrations rodam, schema fica completo.
- Abrir o banco legado atual (`limiar.db`) → `ensureCurrentSchemaCompatibility` adiciona colunas faltantes.
- Não quebrar registros existentes.

---

## CHECKLIST FINAL

Antes de declarar pronto, confirmar:

- [ ] `go build ./...` passa
- [ ] `go vet ./...` limpo
- [ ] `go test ./...` passa
- [ ] `go test -race ./...` sem races
- [ ] Migration 008 criada e registrada em `currentSchemaColumns`
- [ ] `001_initial.sql` atualizado com colunas novas
- [ ] `model.NormalizedMessage` e `model.ProcessedMessage` têm os campos novos
- [ ] `storage.ProcessorRepository` lê/escreve as novas colunas
- [ ] Nenhum import fora da closed stack
- [ ] `internal/processor` não importa `internal/telegram`
- [ ] Funções de extração são puras (determinísticas, sem I/O, sem `time.Now`)
- [ ] Casos de teste do spec cobertos
- [ ] `docs/specs/EXTRACTION-CRE.md` e ADR 014 permanecem coerentes com o implementado
