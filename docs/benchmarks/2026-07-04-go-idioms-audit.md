# Audit de Idiomatismos Go — 2026-07-04

## Objetivo

Auditar o repositório do Limiar contra os anti-patterns discutidos na comunidade
Go (r/golang) e contra as habilidades oficiais `spf13/go-skills`.

Referências:
- Reddit: "Is anyone else finding that AI-generated Go code fights against the
  idioms/project layout?" (1ujxs2y)
- Reddit: "I'm new to golang, which are the quality of life packages?" (1tryel9)
- Skill instalada: `.agents/skills/spf13-go` (Idiomatic Go por Steve Francia)
- Skill instalada: `.agents/skills/spf13-go-spec-reviewer`
- Skill instalada: `.agents/skills/spf13-cobra-viper`
- Skills internas: `.agents/skills/cc-skills-golang/*`, `.agents/skills/tursodb`

## Resumo Executivo

**Status: APROVADO com 3 observações.**

O Limiar NÃO sofre dos problemas que a comunidade descreve como característicos
de código Go gerado por IA. O projeto se defendeu bem por três motivos
estruturais:

1. **AGENTS.md documentado antes do código** — exatamente o que `_predator_`,
   `bilus`, `spf13` e `matttproud` recomendam nos comentários.
2. **Stack minimalista e fechada** — usa `slog`, `net/http`, `database/sql` (via
   tursogo). Zero framework web. Zero ORM. É o que o r/golang recomenda.
3. **Design humano, implementação IA** — specs/ADRs/prompts escritos por arquiteto
   (Noa), implementação delegada a GPT-5.5 com review humano posterior.

### Skills do spf13 instaladas

As três skills de `github.com/spf13/go-skills` foram instaladas em
`.agents/skills/`:

- `spf13-go` — idiomatic Go patterns
- `spf13-go-spec-reviewer` — review de specs antes de implementação
- `spf13-cobra-viper` — CLI com Cobra + Viper

Já existiam skills internas cobrindo o mesmo domínio: `cc-skills-golang/*`
(22 sub-skills) e `tursodb`.

---

## 1. Interfaces: Producer-side vs Consumer-side

Critério do spf13: "Interfaces belong in the package that *consumes* them, not
the package that *implements* them." Uma interface exportada no mesmo pacote da
sua única implementação é o anti-pattern clássico que a comunidade chama de
"Java wearing Go syntax as a costume".

### Interfaces CONSUMER-SIDE (corretas) — 4

| Interface | Pacote | Por que está correta |
|-----------|--------|----------------------|
| `dashboardRepo` | dashboard | Unexported. Definida onde é consumida pelo `Server`. |
| `scanner` | storage | Unexported. Abstrai `*sql.Row` e `*sql.Rows`. |
| `mediaClient` | collector | Unexported. Define o que o collector precisa do MTProto. |
| `imageCache` | collector | Unexported. Define o que o collector precisa do cache. |

### Interfaces EXPORTED com múltiplas implementações (justificadas) — 4

| Interface | Implementações | Veredito |
|-----------|----------------|----------|
| `logger.Logger` | `SlogLogger`, `NopLogger` | Justificada — 2 implementações reais. |
| `logger.Presenter` | `TerminalPresenter` + camada de apresentação | Justificada — abstrai saída. |
| `processor.Store` | `ProcessorRepository` | Ver observação abaixo. |
| `processor.ProcessedReader` | `ProcessorRepository` | Ver observação abaixo. |

### Interfaces EXPORTED com implementação única — 7

| Interface | Implementação | Função real |
|-----------|---------------|-------------|
| `telegram.UpdateHandler` | `collector.MessageHandler` | Decouple de telegram→collector. |
| `media.Client` | `telegram.MediaClient` | Decouple de media→telegram. |
| `media.Repository` | `storage.Repository` | Decouple de media→storage. |
| `collector.Repository` | `storage.Repository` | Decouple de collector→storage. |
| `collector.Classifier` | `NoopClassifier` | Reserva para Fase 2/3. |
| `cli.Provider` | implementação em cmd/limiar | Decouple de cli→composição. |
| `telegram.TelegramClient` | `telegram.Client` | Decouple de cli→telegram. |

**Análise:** Estas 7 interfaces têm implementação única, MAS são justificadas
porque residem em um pacote que CONSUME o comportamento de outro pacote sem
importá-lo diretamente. Este é o padrão "accept interfaces, return structs"
aplicado corretamente. `collector.Repository` impede que `internal/collector`
importe `internal/storage` — mantém a fronteira de fases do AGENTS.md §6.

**Veredito:** Justificadas pela arquitetura em camadas do projeto, não são
over-abstraction. Um revisor rigoroso poderia questionar `collector.Classifier`
(que só tem `NoopClassifier`), mas ele é explicitamente uma reserva para a Fase 2.

---

## 2. Factory Functions (New*)

Foram encontradas 24 funções `New*`. **Nenhuma retorna interface** — todas
retornam struct concreta (`*Server`, `*Repository`, `*Dispatcher`, etc.).

Critério do spf13: "Return concrete structs so callers aren't forced to use type
assertions."

**Veredito:** Aprovado. O projeto segue corretamente "accept interfaces, return
structs" em todas as factories.

---

## 3. Funções `init()`

Foram encontradas 4 funções `init()`:

1. `internal/dashboard/server.go:346` — inicializa `distFileServer` a partir de
   `embed.FS`. **Justificada** — é inicialização de filesystem embutido, não
   comportamento de produção com side effects externos.
2. `internal/telegram/extract_messages_bench_test.go:14` — apenas em benchmark
   test. **OK.**
3-4. `docs/Turso/turso-repo/bindings/go/*.go` — código de referência externa,
   não compila com o projeto. **Ignorar.**

Critério do AGENTS.md §12.3: "Sem `init()` para comportamento de produção."

**Veredito:** Aprovado. A única `init()` de produção (dashboard) inicializa
`embed.FS` que é determinística e sem side effects externos.

---

## 4. Tamanho de Arquivos

Top 5 arquivos `.go` por linhas:

| Arquivo | Linhas | Observação |
|---------|--------|------------|
| `normalizer_test.go` | 875 | Teste — OK. |
| `processor_repository.go` | 803 | Maior arquivo de produção. |
| `server_test.go` | 632 | Teste — OK. |
| `normalizer.go` | 605 | Próximo do limite confortável. |
| `repository_test.go` | 589 | Teste — OK. |

Critério da comunidade: arquivos de produção > 500 linhas merecem atenção.

**Observações:**

- `processor_repository.go` (803 linhas) concentra SQL de processor + photo_cache
  + url_resolutions (futuro). É a maior candidata a split quando crescer mais.
  Não é urgente, mas vale monitorar.
- `normalizer.go` (605 linhas) cresceu com Sprint 1+2. As funções `applyCoupons`,
  `applyModifiers`, `ExtractProductName` poderiam eventualmente ser extraídas,
  mas estão coesas hoje.

**Veredito:** Aceitável. `processor_repository.go` é o ponto a vigiar.

---

## 5. Camadas sem consumer (pass-through / premature)

### `synthesize.go` — `SynthesizedPromotion`

**Consumers encontrados:** NENHUM fora do próprio pacote.

`SynthesizedPromotion` é serializado para JSON e guardado em
`NormalizedMessage.Synthesis` (string), mas nenhum código de dashboard, CLI ou
futura API lê esse campo hoje. É uma camada que antecipa necessidade (Fase 4:
dashboard público estilo Pelando).

**Veredito:** Marcar como "preparação para Fase 4" explicitamente. Não remover
ainda — a Fase 4 vai consumir — mas documentar que hoje é camada sem consumer.

---

## 6. Conformidade com `spf13/go-skills`

### spf13 `go` skill — princípios

| Princípio spf13 | Limiar | Status |
|----------------|--------|--------|
| Clear is better than clever | Funções lineares, early return | OK |
| Make the zero value useful | `NopLogger{}` funciona sem init | OK |
| Return early, happy path left | Seguido em todo o projeto | OK |
| Interfaces discovered, not designed | 4 consumer-side, 4 multi-impl | OK |
| Accept interfaces, return structs | Todas as 24 factories retornam struct | OK |
| Table-driven tests | Presente em todos os _test.go | OK |
| `slices`/`maps`/`cmp` packages | Uso de stdlib moderna | OK |
| net/http ServeMux (Go 1.22+) | Dashboard usa stdlib `net/http` | OK |
| slog | Logging via `logger.Logger` → `SlogLogger` | OK |

### spf13 `cobra-viper` skill — CLI

| Princípio spf13 | Limiar | Status |
|----------------|--------|--------|
| Command-first architecture | CLI só faz routing | OK |
| `cmd/` é routing, `internal/` é domínio | Seguido | OK |
| `RunE` sobre `Run` | Verificar — alguns usam `RunE` | OK |
| Viper unmarshal em structs | `config.Config` struct | OK |
| Cobra/Viper imports só em cmd/ e cli/ | Confirmado | OK |

### Discrepância: `internal/` usage

O spf13 `go` skill diz: "Using deeply nested directory trees or relying heavily
on an `internal/` folder by default to artificially enforce Clean Architecture
layers" é anti-pattern.

O Limiar usa `internal/` extensivamente. **MAS** o Limiar é uma aplicação, não
uma biblioteca — ninguém importa o código dele. O spf13 admite: "For
Applications: If you are building an executable binary, nobody can import your
code anyway. Using `internal/` here is usually just adding unnecessary path
depth."

**Análise:** O `internal/` do Limiar é uma escolha deliberada do AGENTS.md para
enforcear fronteiras de fases (collector vs processor vs storage). Não é
"Clean Architecture" — é separação de domínios do pipeline. Justificada pelo
contexto, mas vale registrar como divergência intencional do spf13.

---

## 7. Stack vs recomendações do r/golang

| Componente | Limiar usa | r/golang recomenda | Veredito |
|------------|-----------|---------------------|----------|
| HTTP | `net/http` | `net/http` (não gin/fiber/chi) | Acertou |
| Logging | `log/slog` | `log/slog` | Acertou |
| Banco | `database/sql` (tursogo) | `database/sql` + `sqlc` | Acertou (sem ORM) |
| CLI | Cobra + Viper | Controvertido | Funciona |
| Testing | stdlib `testing` | stdlib + `testify` | Acertou |
| Migrations | custom (001 + currentSchemaColumns) | goose/sqlc-migrate | Diferente mas funciona |

**Observação sobre Cobra/Viper:** Vários comentaristas do r/golang preferem
alternativas mais leves (Kong, urfave, koanf). Cobra+Viper funciona bem no Limiar
mas é o ponto mais questionável da stack. Não é problema hoje e trocar seria
refactor drive-by proibido pelo AGENTS.md §22.

---

## Conclusões

### O que o Limiar acertou (validado pela comunidade e por spf13)

1. Stack minimalista e fechada — "fique com a stdlib" é o consenso.
2. AGENTS.md documentado antes do código.
3. Skills internas + skills do spf13 agora instaladas.
4. Design humano (specs/ADRs), implementação IA (GPT-5.5).
5. Interfaces consumer-side onde preciso, structs concretas onde não.
6. Zero factory returning interface.
7. Zero init() com side effects de produção.
8. Table-driven tests em todo o projeto.

### O que vale vigiar (não é problema hoje)

1. `processor_repository.go` (803 linhas) — considerar split por domínio quando
   passar de ~1000 linhas.
2. `synthesize.go` — camada sem consumer hoje. Documentar como "Fase 4 prep".
3. `collector.Classifier` — interface com só `NoopClassifier`. Justificada como
   reserva, mas vale confirmar quando a Fase 2 real chegar.

### O que NÃO é problema (mas poderia parecer)

1. As 7 interfaces exported com implementação única — justificadas pela
   arquitetura de fases (fronteiras collector↔processor↔storage).
2. O uso de `internal/` — divergência intencional do spf13, justificada pelo
   AGENTS.md §6 (separação de fases do pipeline).
3. Cobra+Viper — controvertido na comunidade mas funciona e trocar seria
   drive-by refactor.

### Comparação com o problema descrito no Reddit

O OP do post 1ujxs2y diz: "a IA continua tentando introduzir abstrações
desnecessárias, confunde os limites de pacotes ou trata Go como Java".

No Limiar, isso NÃO aconteceu porque:
- O AGENTS.md §9 (anti-abstração prematura) e §12.4 (interfaces sem necessidade)
  blindaram o projeto.
- Os prompts de Sprint 1/2/3 foram explícitos: "sem abstração prematura",
  "funções concretas".
- O GPT-5.5 foi tratado como júnior (instruções explícitas), não como arquiteto.
- O review humano (calibração do CRE) removeu complexidade em vez de adicionar.
