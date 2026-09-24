# PROMPT 2/2 — Implementação da Auditoria de Qualidade Incremental (Limiar)

## Papel

Você é um engenheiro Go sênior. Sua tarefa é IMPLEMENTAR as correções do
relatório de auditoria de qualidade incremental. Você não descobrirá novos
problemas — você resolve os problemas já identificados.

Trate código como arte: cada correção deve deixar o código mais limpo,
mais claro, mais idiomático. Mas arte no sentido Go — simples, direto,
coeso. Não Java over-engineered.

## Fluxo de trabalho

1. Leia o relatório de auditoria (Prompt 1) que o usuário vai te fornecer.
2. Leia as instruções abaixo.
3. Implemente APENAS os achados marcados como "FAZER AGORA" no relatório.
4. Avalie os marcados como "AVALIAR" e pergunte ao usuário antes de tocar.
5. IGNORE os marcados como "NÃO FAZER".

## Leitura obrigatória ANTES de implementar

Carregue e leia, na ordem:

1. `AGENTS.md` — regras rígidas (autoridade máxima)
2. `.agents/skills/spf13-go/SKILL.md` — idiomatic Go
3. `.agents/skills/spf13-cobra-viper/SKILL.Rmd` — CLI conventions
4. `.agents/skills/tursodb/SKILL.md` — Turso/tursogo (se mexer em storage)
5. `docs/adr/` — decisões tomadas (não contradiga)
6. `docs/guidelines/STORAGE.md` — regras de banco
7. O relatório de auditoria do Prompt 1.

Se qualquer skill não estiver disponível, REGISTRE e pare.

## Princípios de implementação

### O que FAZER

- **Menor mudança possível** (AGENTS.md §22).
- Preserve o estilo existente do arquivo que está tocando.
- Adicione imports/dependências APENAS se sua correção exigir.
- Use stdlib moderna (Go 1.21+): `slices`, `maps`, `cmp`, `max`, `min`.
- Use `strings.Builder` para concatenação de strings.
- Use `any` em vez de `interface{}`.
- Use `for range n` (Go 1.22+) em vez de `for i := 0; i < n; i++`.
- Use table-driven tests para qualquer novo teste.
- Funções de teste usam `t.Helper()` quando são helpers.

### O que NÃO FAZER

- **Não crie subpacotes.** Coisas que trabalham juntas ficam juntas.
- **Não crie interfaces.** Se precisar de mock, crie fake manual.
- **Não crie abstrações prematuras.** Uma struct concreta resolve.
- **Não troque dependências.** Cobra, Viper, tursogo — ficam.
- **Não mexa em arquitetura.** Fronteiras de fases são invariantes.
- **Não refatore drive-by.** Só toca o que a auditoria pediu.
- **Não adicione código "por precaução".** YAGNI.
- **Não expanda escopo.** Se a auditoria não pediu, não faça.
- **No panic() em produção** (só no recover do dispatcher).
- **Sem `init()` para comportamento de produção** (AGENTS.md §12.3).
- **Não silencie erros com `_ =`** sem justificativa.

### Regras de estilo do projeto

- Comentários e strings visíveis ao usuário em Português (pt-BR).
- Mensagens de log usam prefixos de emoji: 📡 📩 📜 🔄 ❌ ✅ 🛑 ⏰ 🌐 ⚠️
- Mensagens de erro seguem formato `"layer: op: cause"`.
- Chaves sensíveis (api_hash, session, token, password, auth_code) são mascaradas.
- Preços são INTEGER (centavos). Nunca float64 para valor monetário.
- SQL só em `internal/storage/`. Placeholders são `?`.

## Regras para categorias específicas

### Dead code removal

- Remova a função/type/var/campo.
- Remova imports que ficaram órfãos.
- Rode `goimports` se disponível, senão remova manualmente.
- Se o dead code estiver em um arquivo _test.go, verifique se não é
  helper de teste usado por outros testes no pacote.

### Duplicação

- Antes de consolidar duplicação, CONFIRME que os dois blocos fazem
  exACTAMENTE a mesma coisa. Pequenas diferenças podem ser intencionais.
- Se consolidar, crie uma função auxiliar local (unexported) no mesmo pacote.
- Não crie pacote `utils/` ou `helpers/` para a função consolidada.

### Funções grandes

- Quebre em funções auxiliares (unexported) no mesmo arquivo.
- Mantenha a função pública com a mesma assinatura.
- Não mova as funções auxiliares para outro arquivo sem necessidade.
- Nomeie as auxiliares pelo que fazem, não pela função pai.

### Edge cases e error handling

- Toda função de I/O, banco, rede, ou bloqueante recebe `context.Context`.
- Erros atravessando camadas usam `errors.Wrap(layer, op, err)` ou
  `fmt.Errorf("layer: op: %w", err)`.
- Sentinels residem em `internal/errors/errors.go`.
- Sem `fmt.Errorf("%s: %v", ...)` — sempre `%w` para preservar causa.
- Loops longos verificam `ctx.Err() != nil` ou `ctx.Done()`.

### Performance (só se a auditoria tiver benchmark)

- Reduza alocações só se houver benchmark mostrando gargalo.
- Pre-aloque slices quando o tamanho é conhecido: `make([]T, 0, n)`.
- Use sync.Pool apenas para objetos caros de criar.
- Elimine queries N+1.
- Use prepared statements para escritas recorrentes (AGENTS.md §13.7).

### Memory leak, GC e alocações

- **Nunca deixe goroutines sem exit path**: todo `go func()` precisa de condição de saída clara (context.Context ou canal fechado). Goroutine leak = memory leak + CPU leak.
- **Limpe maps/slices que crescem**: caches devem ter TTL e eviction. `photo_cache` tem TTL de 30 dias — garanta que `CleanExpiredPhotoCache` roda periodicamente no run loop.
- **Pre-aloque slices em hot path**: `make([]T, 0, knownSize)` evita realocações.
- **Use `strings.Builder`** para concatenação de strings em loops (já prática no projeto).
- **Evite `defer` em loops longos sem closure**: `defer` acumula chamadas. Use função auxiliar ou mova o defer pra fora do loop.
- **Não retenha structs grandes desnecessariamente**: se só precisa de um campo, passe o campo, não a struct inteira. Passar por ponteiro se a struct for grande.
- **Evite copiar slices desnecessariamente**: `copy(dst, src)` ou re-slice `s[i:j]` em vez de `append` criando cópia.
- **`sync.Pool` só para objetos caros**: objetos pequenos/criados frequentemente em hot path podem usar pool. Objetos simples não.
- **`context.Context` em operações longas**: permite GC de request scope quando ojects tied ao request quando o context cancela.
- **Feche resources explicitamente**: `io.Closer` (DB connections, HTTP responses, files) — use `defer` fora de loops.
- **Evite `runtime.SetFinalizer`**: raramente necessário e fácil de usar errado. Prefira cleanup explícito.

### CLI UX (apenas internal/cli/)

- Use cores via `lipgloss` SE já estiver no projeto. Senão, use ANSI codes
  direto ou mantenha o estilo atual.
- Spinners via `spinner` SE já estiver no projeto.
- Se a auditoria não pediu visual change específica, não invente.
- Alinhamento: use `text/tabwriter` para colunas.
- Prefixo de emoji nas mensagens de log. Mensagens ao usuário diretas
  sem prefixo de emoji (emoji é pra log, não pra output de CLI).

## Verificação obrigatória

Após implementar TODAS as correções:

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Todas precisam passar. Se uma correção quebrar testes, você tem duas opções:
1. O teste estava testando comportamento errado → corrija o teste.
2. Sua correção está errada → reverta e reporte o bloqueador.

Build não deve conter imports de `sqlite`, `mattn`, `gorm`.

## Verificação de memory/GC

Após implementar, execute estas verificações adicionais:

```sh
# Teste de leak de goroutine (deve completar sem travar)
go test -race -run TestNoGoroutineLeak ./... 2>&1 | head -20

# Verificação de alocações em hot path (se houver benchmarks)
go test -bench=BenchmarkNormalize -benchmem ./internal/processor/... 2>&1 | grep -E "Benchmark|allocs|bytes"

# Verificação de GC: execute reprocessamento completo e monitore RSS
# RSS não deve crescer linearmente com o número de mensagens processadas
```

## Verificação de invariantes

Confirme após implementar:

- `internal/processor/` não importa `internal/telegram` nem `gotd/td`
- `internal/dashboard/` não importa `internal/telegram`
- `internal/dashboard/` não escreve em banco
- SQL continua centralizado em `internal/storage/`
- Collector não processa conteúdo (não normaliza, não classifica)
- Preços continuam INTEGER (centavos)

## Entregável

1. Código corrigido nos arquivos tocados.
2. Testes atualizados para qualquer comportamento que mudou.
3. Resumo final contendo:
   - Quantos achados foram implementados.
   - Quantos foram avaliados e adiados (com motivo).
   - Resultado de `go build`, `go vet`, `go test`, `go test -race`.
   - Lista de arquivos modificados.
   - Qualquer bloqueador encontrado.

## Anti-instruções

- NÃO crie ADRs. Auditoria não é mudança arquitetural.
- NÃO reorganize pacotes.
- NÃO crie subpastas.
- NÃO mude nomenclatura de exported types sem permissão explícita.
- NÃO adicione novas dependências.
- NÃO mexa em migrações SQL ou schema.
- NÃO altere o AGENTS.md ou ADRs.
- NÃO implemente achados marcados como "NÃO FAZER".
- NÃO expanda escopo por iniciativa própria.
