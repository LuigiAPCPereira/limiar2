# ADR 014 — Extração Determinística e Candidate Ranking Engine

## Status

Proposto

## Contexto

O `limiar-processor` (Fase 2) implementa extração de sinais via regex em
`internal/processor/normalizer.go`. Uma análise do banco de produção
(31.832 mensagens processadas, 15 canais, realizada em 2026-07-03) revelou
três categorias de sub-extração e uma funcionalidade ausente:

### Problemas medidos (baseline de 31.832 mensagens)

| Problema | Medido | Detalhe |
|----------|--------|---------|
| Cupons múltiplos perdidos | ~22% das msgs com cupom | `extractCoupon` usa `FindStringSubmatch` (first-only). Padrões " ou " e " + " documentados no spec mas não implementados |
| PIX sub-extraído | 24% de loss (1.305 de 5.417) | Regex não cobre "(pix)", "SELECIONE PIX NA PAG", "pix na pag de pagamentos" |
| VirtualCurrency (Moedas) | 100% de loss (1.699 msgs) | Spec §10.3 prevê `VirtualCurrency`, código não implementa |
| App-only / Web-only | 100% de loss (715 + 573 msgs) | Modifiers de canal de venda não detectados |
| `product_name` | 0% de coverage | Campo existe no schema, sempre vazio. Spec atual atribui ao LLM (Fase 3) |

### A questão do ProductName

O `PROCESSOR-IDEATION.md` e o `synthesize.go` afirmam que extração de
`ProductName` "requer LLM (Fase 3)". A análise das amostras de `deal_complete`
revelou que **em ~70-80% das mensagens, o nome do produto é a primeira linha
limpa do texto**, após remover modificadores e meta-anúncios. Exemplos
verificados:

```
"Anker Caixa de Som Soundcore Select 4 go"     ← linha 1
"Smartphone Motorola Razr 60-256GB 24GB..."    ← linha 1
"Monitor Gamer SuperFrame Enterprise, 34 Pol"  ← linha 1
"Apple iPhone 16 (128 GB)"                     ← linha 1
```

Isso indica que uma heurística determinística pode cobrir a maioria dos
casos sem invocar LLM, invertendo a relação do spec: a heurística vira a
abordagem primária e o LLM vira o fallback para casos de baixa confiança.

### Webpage.Title como fonte

A análise confirmou que `Media.Webpage` está presente em apenas 0.31% dos
payloads (96 de 31.832). Embora seja gold standard quando existe, sua
cobertura é insuficiente para ser fonte primária. É extraída como bônus
quando disponível, sem depender dela.

## Constraints

- **AGENTS.md §11**: stack fechada. Nenhuma dependência externa nova.
  Extração é regex + scoring (stdlib).
- **AGENTS.md Invariante 4**: Processor é determinístico. Mesmo input →
  mesmo output. A heurística satisfaz isso; LLM não.
- **AGENTS.md Invariante 3**: Reprocessamento deve ser possível a partir de
  raw + config + código. Heurística satisfaz; LLM é frágil (modelo deprecia).
- **AGENTS.md §16**: Toda inferência de IA precisa de confidence score,
  rastreabilidade e reprocessamento. A heurística gera score naturalmente.
- **AGENTS.md §9**: Anti-abstração prematura. Não criar motor genérico sem
  necessidade comprovada. Mas: separação de responsabilidades e funções
  bem-testadas é modularidade, não abstração prematura.
- **AGENTS.md Invariante 6**: Simplicidade vence sofisticação. Menos
  componentes, menos abstrações, menos dependências.
- **AGENTS.md §4**: Protocolo de raciocínio. Menor solução que funciona.
- **AGENTS.md §15.1**: Processor nunca importa `internal/telegram`. LLM
  fica na Fase 3.

## Alternativas consideradas

### 1. Motor genérico de candidate ranking para todos os problemas

Criar uma abstração `CandidateRanker[T]` que serve para cupons, modifiers
e product_name.

**Rejeitada.** Os três problemas têm formatos diferentes:
- Cupons múltiplos → **enumeração** (quer-se TODOS, não o "melhor")
- Modifiers → **extração estruturada** (quer-se uma lista de presentes)
- ProductName → **ranking** (quer-se o melhor candidato entre vários)

Forçar os três num motor único viola Invariante 6 e cria acoplamento entre
problemas não-relacionados. Cada formato tem a solução certa no seu tamanho.
Abstrair depois que 3+ instâncias concretas existam (Rule of Three).

### 2. Pular direto para LLM (Fase 3) para ProductName

**Rejeitada.** Viola a filosofia do projeto. A heurística satisfaz mais
invariantes que o LLM (determinismo, reprocessamento, sem dependência de
API externa). Além disso, reduz custo: LLM roda apenas nos ~30-45% difíceis.
E gera uma baseline que serve como pré-label para few-shot do LLM depois.

### 3. URL Resolver (Estágio 2) antes de tudo

**Rejeitada como pré-requisito, aceita como complemento.** O URL Resolver
é a peça mais cara do processor (rede, rate limiting, anti-bot, cache
persistente). Implementá-lo ANTES de medir se a heurística de texto cobre
o suficiente é decisão sem evidência. Entra como uma das fontes do CRE
(Sprint 3), não como bloqueador.

### 4. Extração determinística em camadas (ESCOLHIDA)

Três melhorias separadas, cada uma no formato certo, com código modular e
testável. ProductName usa candidate ranking de verdade. URL Resolver entra
como fonte complementar e também como limpador de affiliate links.

## Decisão

Adotar extração determinística em três sprints, com ProductName usando
candidate ranking engine:

### Sprint 1 — Correções de extração (cupons + modifiers)

- **Cupons**: função `extractCoupons` (plural) que retorna `[]Coupon`
  estruturado. `FindAll` + split por " ou " e " + ". Schema adiciona
  `coupon_codes` (JSON array) mantendo `coupon_code` (primeiro) para compat.
- **Modifiers**: struct `Modifier { Type, Value }` + função
  `extractModifiers` que retorna lista. Adiciona regex para app-only,
  web-only, VirtualCurrency. Refina regex PIX. Schema adiciona `modifiers`
  (JSON array).
- **Código modular**: cada extractor é uma função pura (input texto → output
  estruturado), coesa, testável isoladamente. Sem framework, sem interface
  prematura — funções bem nomeadas com responsabilidade única.

### Sprint 2 — Candidate Ranking Engine para ProductName

- Arquitetura modular com fontes de candidatos plugáveis e scorers
  independentes, mas começando com instâncias concretas (não abstração
  hipotética).
- Cascata de fontes: (1) Webpage.Title se existir, (2) heurística de texto
  com scoring, (3) URL resolver `<title>` quando implementado (Sprint 3).
- Score normalizado 0.0–1.0. Abaixo do threshold → campo vazio, marcado
  para LLM (Fase 3).
- Confidence armazenado em coluna nova `product_name_confidence REAL`.

### Sprint 3 — URL Resolver (Estágio 2)

- Resolve URL canônica (follow redirects, remove tracking params).
- Extrai `<title>` da página final (fonte complementar para CRE).
- Limpa affiliate links (substitui por canonical no campo `url`).
- Cache persistente em `url_resolutions(original, canonical, merchant)`.
- (Futuro) Re-afiliação com link próprio — estudo separado, fora do escopo
  imediato.

## Justificativa

A alternativa 4 vence porque:

1. **Respeita invariantes**: cada peça é determinística, reprocessável e
   não adiciona dependência externa.
2. **Baseada em evidência**: a análise do banco provou que a primeira linha
   cobre ~70-80% dos ProductNames. Não é especulação.
3. **Modular sem over-engineering**: funções puras e bem testadas são
   modulares por natureza. Não precisa de framework genérico.
4. **Evolução incremental**: Sprint 1 é barato e mensurável. Sprint 2 é a
   peça substantiva. Sprint 3 só se a evidência pedir.
5. **Reduz dependência de LLM**: de 100% para ~30-45%, com baseline que
   melhora futuros prompts few-shot.
6. **Não viola Invariante 6**: menos abstrações que motor genérico, menos
   componentes que pular pra LLM, menos dependências que URL Resolver cedo.

## Verificação

### Sprint 1
- [ ] `go build ./...` passa
- [ ] `go vet ./...` limpo
- [ ] `go test ./internal/processor/...` passa com casos de cupom múltiplo
- [ ] `go test -race ./...` sem data races
- [ ] Re-processar banco: % de msgs com `coupon_codes` não-vazio aumenta
- [ ] Re-processar banco: % de msgs com `payment_method='pix'` sobe de 12.9%
- [ ] Migração idempotente em banco novo e legado

### Sprint 2
- [ ] `go build ./...` passa
- [ ] `go test ./internal/processor/...` com casos de ProductName
- [ ] Re-processar banco: % de `deal_*` com `product_name != ''` ≥ 50%
- [ ] Confidence score preenchido em toda msg com product_name
- [ ] Casos de baixa confiança ficam vazios (não inventa nome)

### Sprint 3
- [ ] URL resolver não bloqueia o pipeline (timeout + fallback gracioso)
- [ ] Cache `url_resolutions` persiste e é reutilizado
- [ ] Affiliate tracking params removidos do campo `url`
- [ ] `<title>` alimenta o CRE quando disponível
- [ ] Rate limiting respeitado (Shopee P1, Amazon P1)

## Dependências

- Nenhuma dependência externa nova (AGENTS.md §11).
- Sprint 3 usa `net/http` da stdlib (já permitido para dashboard; novo uso
  no processor requer nota de que é rede outbound controlada).
- Migrações de schema seguem `docs/guidelines/STORAGE.md`.

## Riscos

- **Falsos positivos de ProductName**: meta-anúncios ("NOVO CUPOM AMAZON")
  podem ser classificados como nome. Mitigado por penalização no scoring e
  threshold de confiança.
- **Cobertura < esperada**: se a heurística cobrir < 50%, Sprint 3 (URL
  resolver) ganha prioridade. Medido após Sprint 2.
- **Rate limiting no Sprint 3**: merchants podem bloquear. Mitigado por
  cache agressivo, backoff exponencial e User-Agent real.
- **Mudança de formato dos canais**: canais mudam padrão de texto. A
  análise é um snapshot. Mitigado por código extensível e reprocessável.
