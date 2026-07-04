# PROMPT — Sprint 2: Candidate Ranking Engine (ProductName)

## O QUE ESTÁ ACONTECENDO

Você está atuando como engenheiro sênior no projeto **Limiar** (`github.com/limiar/collector`), um pipeline Go que captura promoções de canais do Telegram e as processa em dados estruturados.

Este é o **Sprint 2 de 3** de uma sequência de extração determinística. O **Sprint 1 já foi concluído** (cupons múltiplos, modifiers estruturados, VirtualCurrency, Webpage.Title/URL/Desc persistidos).

**Este prompt foca APENAS no Sprint 2 (CRE).** Não implemente o Sprint 3 (URL Resolver).

---

## LEITURA OBRIGATÓRIA ANTES DE CODAR

Você tem acesso ao repo. Leia estes arquivos antes de tocar em qualquer código:

1. **`AGENTS.md`** — Regras obrigatórias, invariantes arquiteturais e stack fechada.
2. **`docs/specs/EXTRACTION-CRE.md`** — A especificação COMPLETA deste Sprint 2. Siga a seção "Sprint 2" rigorosamente. **ATENÇÃO especial à seção 2.8**: a migration usa a estratégia de marcador (igual ao Sprint 1), NÃO `ALTER TABLE` direto.
3. **`docs/adr/014-deterministic-extraction-and-cre.md`** — Contexto arquitetural.
4. **`docs/guidelines/STORAGE.md`** — Regras de migrations (leia a regra 7 sobre estratégia consolidada + reparo).
5. **`internal/processor/normalizer.go`** — Onde o CRE será integrado (após `extractModifiers`).
6. **`internal/processor/synthesize.go`** — Confirme que o comentário não atribui `ProductName` exclusivamente ao LLM.
7. **`internal/model/normalized.go`** e **`internal/model/processed.go`** — Onde vivem as structs.

---

## O PROBLEMA

O campo `product_name` está vazio em 100% das 31.832 mensagens processadas. O spec original atribuía isso à Fase 3 (LLM). Mas a análise do banco mostrou que em **~70-80% das mensagens `deal_*`, o nome do produto é a primeira linha limpa do texto**.

O objetivo do CRE é extrair `product_name` deterministicamente (sem LLM), com score de confiança. A heurística vira a fonte primária; o LLM vira o fallback para os casos difíceis (score abaixo do threshold).

---

## ESCOPO DA TAREFA (Sprint 2)

Implementar o **Candidate Ranking Engine (CRE)** conforme a seção "Sprint 2" de `docs/specs/EXTRACTION-CRE.md`. O CRE é um pipeline de **3 estágios** fundamentado em literatura de Attribute Value Extraction e Two-Stage Retrieval.

### O Pipeline do CRE (3 Estágios)

```
INPUT: NormalizedMessage
    │
    ▼
STAGE 1: CANDIDATE GENERATION
  Fontes geram []Candidate (cada uma com Text, Source, Detail, BaseConfidence):
   - WebpageTitleSource (lê nm.WebpageTitle do Sprint 1, BaseConf 0.60)
   - TextHeuristicSource (divide texto em linhas, gera até 3 candidatos)
   - URLTitleSource (Sprint 3 — ainda NÃO existe, não implementar)
    │
    ▼
STAGE 2: SCORING
  Cada candidato é pontuado por scorers que ajustam a BaseConfidence:
   + PositionScorer      (0.25, linha 0 ganha tudo)
   + CapitalizationScorer (0.20, Title Case > 60%)
   + TechDensityScorer   (0.20, densidade de specs técnicas)
   + BrandMatchScorer    (0.20, marca na lista embed)
   + ProximityScorer     (0.10, perto de preço/cupom)
   - MetaPenaltyScorer   (0.15, meta-anúncio tipo "CUPOM NOVO")

  Score final = clamp(BaseConfidence + somatório_ajustes, 0.0, 1.0)
  Vencedor = maior score
    │
    ▼
STAGE 3: NORMALIZATION (pós-extração)
  Limpa o nome escolhido antes de persistir:
   - Remove merchant suffix (" - Amazon.com.br")
   - Remove SKU trailing ("XYZ-BLU-L-2024")
   - Remove promo noise residual ("BESTSELLER!", "FRETE GRÁTIS")
   - Normaliza casing
   - Collapse whitespace
    │
    ▼
THRESHOLD CHECK:
  score >= 0.45 → product_name = nome limpo, confidence = score
  score <  0.45 → product_name = "", confidence = score (pra LLM futuro)
```

### Entregáveis

1. **`internal/processor/cre.go`** — Engine principal (`ExtractProductName`), orquestra os 3 estágios.
2. **`internal/processor/cre_types.go`** — Structs `Candidate`, `ProductNameResult`.
3. **`internal/processor/cre_sources.go`** — Stage 1: `WebpageTitleSource`, `TextHeuristicSource`. (Não implemente URLTitleSource.)
4. **`internal/processor/cre_scorers.go`** — Stage 2: os 6 scorers.
5. **`internal/processor/cre_normalize.go`** — Stage 3: normalização pós-extração.
6. **`internal/processor/cre_data.go`** — `embed.FS` + `loadEmbedList`.
7. **`internal/processor/data/`** — Arquivos `.txt` com as listas (marcas, tech_tokens, merchant_suffixes, promo_noise). **Use nomes extraídos do corpus de 6674/31832 mensagens** (Samsung, Motorola, Apple, Anker, Sony, LG, Xiaomi, HyperX, Logitech, Elgin, etc. para brands; GB, RAM, Hz, pol, LED, OLED, etc. para tech_tokens).
8. **Migração 009** (marcador) + `product_name_confidence` em `currentSchemaColumns` e `001_initial.sql`.
9. **Model atualizado** — `ProductNameConfidence float64` em `NormalizedMessage` e `ProcessedMessage`.
10. **Repository atualizado** — Save/Scan lê/escreve `product_name_confidence`.
11. **Integração** — `normalizer.go` chama `ExtractProductName(nm)` após `extractModifiers`. Atualizar comentário em `synthesize.go`.
12. **Testes** — Um arquivo `_test.go` por componente, com casos reais do corpus.

### Exemplo concreto de scoring (do spec)

Texto:
```
PARCELADO🔥🔥🔥🔥

Anker Caixa de Som Soundcore Select 4 go

POR: 154 REAIS E FRETE GRÁTIS PRIME
```

- Candidato A (line:0): "PARCELADO🔥🔥🔥🔥" → score 0.40 (MetaPenalty mata)
- Candidato B (line:1): "Anker Caixa de Som Soundcore Select 4 go" → score 0.85 (Brand + Title Case)

Vencedor: B. Após Stage 3 → "Anker Caixa de Som Soundcore Select 4 go".

---

## REGRAS RÍGIDAS (AGENTS.md — não negociáveis)

- **Stack Fechada (§11)**: NENHUMA dependência nova. CRE é regex + scoring (stdlib).
- **Determinismo (Invariante 4)**: Mesmo input → mesmo output. Sem `time.Now()`, sem rede, sem random.
- **Processor não importa Telegram**: `internal/processor` nunca importa `internal/telegram` (§15.1).
- **Todo SQL em `internal/storage`**: Placeholders `?` (§13.7).
- **Estratégia de migration (guidelines §7)**: NÃO use `ALTER TABLE ADD COLUMN` direto em `009_*.sql`. Use a estratégia de marcador + `currentSchemaColumns` + `001_initial.sql`.
- **Erros com wrapping**: `"layer: op: cause"` via `apperrors.Wrap` (§12.2).

---

## PRINCÍPIOS DE ENGENHARIA

1. **Alta coesão, baixa acoplamento**: cada scorer e fonte é função pura. Não conhece banco, não conhece telegram.
2. **Extensível sem refatoração**: adicionar marca = editar `.txt`. Adicionar scorer = adicionar função num slice.
3. **Limpo e legível**: nomes descritivos, funções curtas, zero magic numbers. Os pesos dos scorers (0.25, 0.20, etc.) e o threshold (0.45) devem ser **constantes nomeadas**, não literais espalhados.
4. **Testável**: tabela de testes com casos reais do corpus.
5. **Determinístico**: CRE é CPU-only, sem I/O.
6. **Sem abstração prematura**: funções concretas. Não criar interfaces sem 2+ implementações reais.

---

## CASOS DE TESTE OBRIGATÓRIOS

### `cre_test.go`
- **Caso fácil (linha 1 = nome)**: "Anker Caixa de Som Soundcore Select 4 go" como linha 1, com "PARCELADO" na linha 0 → extrai o nome certo, ignora o modificador.
- **Webpage.Title presente**: se `nm.WebpageTitle` tem valor, ele vence (gold standard).
- **Meta-anúncio disfarçado**: "NOVO CUPOM AMAZON" como linha 0 → score < threshold → `product_name` vazio, `confidence` guarda o score baixo.
- **Produto com specs densas**: "Smartphone Motorola Razr 60-256GB 24GB" → TechDensity + BrandMatch aplicam.
- **Threshold boundary**: candidato com score exatamente 0.45 → aceito.

### `cre_sources_test.go`
- **TextHeuristicSource**: gera candidatos de linha 0, linha 0-1, e near_price corretamente.
- **cleanLine**: remove emojis de borda e modificadores isolados.
- **WebpageTitleSource**: retorna candidato quando `nm.WebpageTitle` não vazio; vazio quando não há.

### `cre_scorers_test.go`
- Cada scorer testado isoladamente com inputs que disparam e não disparam o bônus/penalidade.
- **PositionScorer**: linha 0 = bônus total; linha 1 = parcial.
- **BrandMatchScorer**: "Anker" na lista = +0.20; palavra não-marca = +0.00.
- **MetaPenaltyScorer**: "PARCELADO", "CUPOM NOVO" = -0.15.

### `cre_normalize_test.go`
- **Merchant suffix**: "Mouse HyperX - Amazon.com.br" → "Mouse HyperX".
- **SKU trailing**: "T-Shirt XYZ-BLU-L-2024" → "T-Shirt".
- **Promo noise residual**: "BESTSELLER! Produto FRETE GRÁTIS" → "Produto".
- **Casing ALL CAPS**: "MOUSE GAMER" → "Mouse Gamer".

---

## ESTRUTURA DE ARQUIVOS ESPERADA

```
internal/model/
├── normalized.go          (MODIFICADO — ProductNameConfidence)
└── processed.go           (MODIFICADO — ProductNameConfidence)

internal/processor/
├── normalizer.go          (MODIFICADO — chama ExtractProductName após extractModifiers)
├── synthesize.go          (MODIFICADO — atualizar comentário sobre ProductName)
├── cre.go                 (NOVO)
├── cre_types.go           (NOVO)
├── cre_sources.go         (NOVO)
├── cre_scorers.go         (NOVO)
├── cre_normalize.go       (NOVO)
├── cre_data.go            (NOVO)
├── data/                  (NOVO)
│   ├── brands.txt
│   ├── tech_tokens.txt
│   ├── merchant_suffixes.txt
│   └── promo_noise.txt
├── cre_test.go            (NOVO)
├── cre_sources_test.go    (NOVO)
├── cre_scorers_test.go    (NOVO)
└── cre_normalize_test.go  (NOVO)

internal/storage/
├── migrations/009_cre_confidence.sql      (NOVO — marcador)
├── migrations/001_initial.sql             (MODIFICADO — product_name_confidence)
├── migrations.go                          (MODIFICADO — currentSchemaColumns)
└── processor_repository.go                (MODIFICADO — Save/Scan)
```

---

## O QUE NÃO FAZER

- **Não implemente o Sprint 3 (URL Resolver).** O `URLTitleSource` fica como placeholder/não-existente por enquanto.
- **Não adicione dependências externas.**
- **Não crie interfaces prematuras** sem 2+ implementações.
- **Não use `ALTER TABLE ADD COLUMN` no arquivo de migration 009.** Use a estratégia de marcador (ver Sprint 1 e guidelines §7).
- **Não invente campos, APIs ou imports** que não existem no repositório.

---

## VERIFICAÇÃO (deve passar antes de considerar pronto)

```sh
go build ./...           # saída 0, zero erros
go vet ./...             # zero problemas
go test ./...            # todos passam
go test -race ./...      # sem data races
```

**Verificação de schema (idempotência):**
- Banco novo (vazio) → `001_initial.sql` já cria `product_name_confidence`.
- Banco legado (`limiar.db`) → `currentSchemaColumns` adiciona a coluna via `PRAGMA table_info`.
- `009_*.sql` é apenas marcador, não tenta `ALTER TABLE`.

---

## CHECKLIST FINAL

- [ ] `go build ./...` passa
- [ ] `go vet ./...` limpo
- [ ] `go test ./...` passa
- [ ] `go test -race ./...` sem races
- [ ] CRE tem 3 estágios: Candidate Generation, Scoring, Normalization
- [ ] 6 scorers implementados com pesos constantes nomeados
- [ ] Threshold 0.45 como constante nomeada
- [ ] Arquivos embed `data/*.txt` criados com conteúdo real baseado no corpus
- [ ] `cre_data.go` carrega listas via `embed.FS`
- [ ] Migration 009 é marcador (sem ALTER TABLE direto)
- [ ] `product_name_confidence` em `001_initial.sql` E `currentSchemaColumns`
- [ ] `model.NormalizedMessage` e `model.ProcessedMessage` têm `ProductNameConfidence`
- [ ] `storage.ProcessorRepository` lê/escreve `product_name_confidence`
- [ ] `normalizer.go` integra `ExtractProductName` após `extractModifiers`
- [ ] `synthesize.go` atualizado (ProductName agora é extraído pelo CRE)
- [ ] Nenhum import fora da closed stack
- [ ] `internal/processor` não importa `internal/telegram`
- [ ] Funções de scoring são puras (determinísticas, sem I/O)
- [ ] Casos de teste do spec cobertos (incluindo o exemplo do "Anker")
