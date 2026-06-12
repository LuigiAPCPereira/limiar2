# AGENTS.md — Regras obrigatórias para agentes de IA e contribuidores

Este arquivo define as regras rígidas para trabalhar no `limiar-collector` e nos
binários relacionados do projeto `github.com/limiar/collector`.

Estas regras não são sugestões, nem estilo preferido, nem "boas práticas opcionais".
Elas existem para impedir que agentes improvisem arquitetura, ignorem documentação,
reescrevam partes estáveis sem necessidade ou assumam comportamento errado de
bibliotecas externas.

Se uma instrução aqui conflitar com um palpite do modelo, a instrução aqui vence.

---

## 1) Ordem de autoridade

Use esta hierarquia, nesta ordem:

1. AGENTS.md
2. `docs/adr/` (ADRs — decisões já tomadas)
3. `docs/specs/` (TURSOGO.md, GOTD-TD.md)
4. Skills do projeto (`turso-db`, `cc-skills-golang`)
5. Documentação oficial das bibliotecas e fornecedores
6. Código existente no repositório

**Se documentação e código divergirem:**

- Não assumir qual está correto.
- Identificar a divergência.
- Registrar explicitamente.
- Propor a correção.

Nunca inventar uma solução silenciosa para resolver um conflito não compreendido.

---

## 2) Leitura obrigatória antes de qualquer alteração

Antes de alterar qualquer código, leia o conjunto relevante de documentação.

**Sempre consulte primeiro:**

- `docs/ARCHITECTURE.md` — diagrama do pipeline, modelo de concorrência, modelo de dados
- `docs/CONTEXT.md` — visão geral do sistema, limites de escopo, roadmap de fases
- `docs/PRODUCT_BRIEF.md` — objetivos e posicionamento do produto
- `docs/adr/` — registros de decisão de arquitetura
- `docs/guidelines/` — diretrizes de extensão por camada

**Para alterações relacionadas a banco de dados, consulte também:**

- `docs/specs/TURSOGO.md`
- A skill `turso-db`

**Para alterações em Go, consulte também:**

- `docs/specs/GOTD-TD.md`
- A skill `cc-skills-golang` (especialmente: `golang-code-style`, `golang-database`,
  `golang-concurrency`, `golang-context`, `golang-error-handling`, `golang-testing`,
  `golang-project-layout`, `golang-cli`, `golang-security`, `golang-performance`,
  `golang-observability`)

**Regra rígida:** Se uma resposta puder ser obtida em `docs/` ou nas skills aprovadas,
use isso em vez de adivinhar.

---

## 3) Skills obrigatórias

Este projeto possui apenas duas skills externas aprovadas:

- **`turso-db`**
- **`cc-skills-golang`**

Elas não são decorativas. Elas fazem parte do processo de trabalho do projeto.

### Obrigatório usar `turso-db`

A skill `turso-db` deve ser consultada antes de qualquer decisão, alteração, análise
ou refatoração que envolva: Turso Database / Tursogo, SQL, schema, migrações,
transações, concorrência de escrita, leitura de banco, replicação, sync, CDC, MVCC,
encryption at rest, FTS, vector search, DSN / connection string, driver tursogo,
qualquer comportamento de banco local embutido.

### Obrigatório usar `cc-skills-golang`

A skill `cc-skills-golang` deve ser consultada antes de qualquer alteração que envolva:
design Go, concorrência, contexto, testes, observabilidade, performance, segurança,
layout de projeto, CLI, logging, tratamento de erro, dependências.

### Regra de não-pular skill

Não é permitido concluir sobre Turso, Go ou arquitetura sem consultar a skill
apropriada quando o assunto estiver no escopo dela.

### Regra de falha

Se a skill não estiver disponível, carregada ou acessível, pare e registre isso
explicitamente. Não substitua skill por "memória do modelo".

---

## 4) Protocolo de raciocínio obrigatório

Antes de codar, o agente deve fazer o seguinte, explicitamente:

1. Declarar as premissas.
2. Apontar ambiguidades.
3. Dizer o que ainda precisa ser verificado.
4. Escolher a solução mínima.
5. Definir como a solução será verificada.

**Regras do protocolo:**

- Não assuma comportamento de biblioteca sem prova.
- Não silencie dúvidas.
- Se existir mais de uma interpretação plausível, apresente as opções.
- Se a solução puder ser menor, proponha a menor.
- Se algo parecer excessivamente complexo, pare e simplifique.

**Modelo obrigatório de raciocínio para tarefas maiores:**

```
1. [Passo] → verificar: [critério]
2. [Passo] → verificar: [critério]
3. [Passo] → verificar: [critério]
```

Se não houver critério verificável, a tarefa ainda está mal definida.

---

## 5) Decision Record obrigatório

Para qualquer decisão arquitetural não trivial, o agente deve registrar:

### Contexto

O problema que está sendo resolvido.

### Constraints

Restrições documentadas relevantes (ADRs, AGENTS.md, skills).

### Alternativas consideradas

1. ...
2. ...
3. ...

### Decisão

Escolha realizada.

### Justificativa

Por que a alternativa escolhida vence as demais.

### Verificação

Como validar que a decisão está correta.

Se a decisão resultar em mudança permanente, ela deve virar um ADR formal em `docs/adr/`.

---

## 6) Fases do projeto e escopo

O Limiar é um pipeline com fases distintas. Cada componente tem escopo fechado.

### Fase 1 — limiar-collector

**Responsabilidades:**

- Autenticação Telegram (userbot)
- Gerenciamento de canais monitorados
- Captura bruta de mensagens
- Persistência de payload raw como JSON
- Backfill de histórico na primeira execução
- Retomada a partir do cursor
- Sessão, peers e cursor de leitura
- Dashboard leve de inspeção (read-only + SSE)
- Desligamento gracioso
- Logging pretty/text/json

**Fora do escopo da Fase 1 — NÃO implementar aqui:**

Normalização, classificação semântica, deduplicação inteligente, enriquecimento,
chamadas LLM, exportação de métricas, alertas, APIs públicas de dados processados.

### Fase 2 — limiar-processor

**Responsabilidades:**

- Ler `raw_messages` (read-only)
- Normalizar payloads em estrutura canônica
- Classificar por tipo de mensagem
- Deduplicar quando previsto
- Escrever em `processed_messages`

**Entradas:** `raw_messages`
**Saídas:** `processed_messages`

### Fase 3 — limiar-ai (opcional)

Camada opcional de enriquecimento semântico baseada em IA.

**Responsabilidades:**

- Sumarização
- Extração avançada de entidades (produto, marca, merchant)
- Classificação assistida por LLM
- Enriquecimento contextual
- Geração de embeddings
- Recuperação semântica (similarity search)
- Confidence scoring

**Entradas:** `processed_messages`
**Saídas:** `enriched_messages`, embeddings, entidades semânticas

**Regras:**

- O sistema deve continuar funcional sem esta fase.
- Nenhum componente anterior pode depender da existência do LM.
- Collector e Processor não podem chamar LLMs diretamente.
- A ausência do LM não deve impedir o funcionamento da API.

### Fase 4 — limiar-api (futuro)

API pública de consulta a dados processados e enriquecidos.

**Entradas:** `processed_messages` + `enriched_messages` (quando disponível)

### Regra de desvio

Se uma alteração introduzir lógica que pertence a outra fase, pare e documente
o desvio. Não misture responsabilidades entre fases.

---

## 7) Invariantes Arquiteturais

As regras abaixo são invariantes do sistema. Um agente NÃO pode alterá-las sem
atualizar ADRs, documentação e justificar explicitamente a mudança.

### Invariante 1 — O Collector não processa conteúdo

O Collector existe para capturar e armazenar dados. Ele NÃO existe para entender
mensagens. Portanto o Collector:

- não classifica
- não normaliza
- não deduplica semanticamente
- não extrai entidades
- não chama LLMs
- não toma decisões de negócio

O Collector apenas: lê updates, persiste payloads, mantém cursores, gerencia
canais, expõe observabilidade.

Qualquer lógica que tente interpretar significado pertence ao Processor.

### Invariante 2 — Raw é a fonte da verdade

A tabela `raw_messages` é a representação canônica do que foi recebido.

- Nunca sobrescrever payloads.
- Nunca mutar payloads históricos.
- Nunca "corrigir" mensagens capturadas.
- Transformações devem produzir novos registros derivados.

### Invariante 3 — Reprocessamento deve ser possível

Todo dado processado deve poder ser reconstruído a partir de:

- `raw_messages`
- configuração
- código-fonte

Se uma mudança impedir reprocessamento completo, ela deve ser rejeitada.

### Invariante 4 — Processor é determinístico

O Processor transforma dados. Ele não captura dados. Ele não conversa com
Telegram. Ele não altera mensagens brutas.

- Entrada: `raw_messages`
- Saída: `processed_messages`

O mesmo input deve produzir o mesmo output.

### Invariante 5 — O banco é um detalhe físico

O sistema não deve assumir que Collector e Processor podem abrir simultaneamente
o mesmo arquivo. Qualquer desenho que dependa disso deve primeiro validar:

- suporte do driver
- suporte da engine
- configuração ativa
- riscos documentados

Assumir suporte por analogia com SQLite é proibido.

### Invariante 6 — Simplicidade vence sofisticação

Entre duas soluções corretas:

- menos componentes vence
- menos processos vence
- menos goroutines vence
- menos canais vence
- menos abstrações vence
- menos dependências vence

O ônus da prova pertence à solução mais complexa.

---

## 8) Arquitetura alvo do pipeline

```
Collector
    ↓
raw_messages
    ↓
Processor
    ↓
processed_messages
    ├──────────────→ API
    │
    ▼
LM (opcional)
    ↓
enriched_messages
    ↓
API
    ↓
Consumers
```

**Nenhum componente pode pular uma camada.** Exemplos proibidos:

- Collector → processed_messages
- Collector → API
- Processor → Telegram
- Dashboard → Telegram
- Dashboard → escrita em banco
- LM → raw_messages (escrita)
- LM → Telegram

---

## 9) Regras anti-abstração prematura

Antes de propor uma nova abstração, responder:

1. Quantas implementações existem hoje?
2. Quantas implementações existem no roadmap?
3. O problema já existe ou é hipotético?
4. O código atual realmente sofre com isso?

Se a resposta for "1 implementação" e "0 necessidade comprovada", não criar a
abstração.

---

## 10) Sinais de arquitetura suspeita

Antes de implementar, pare e reavalie se a solução contém:

- Mais abstrações do que implementações.
- Mais interfaces do que structs.
- Mais processos do que responsabilidades.
- Mais goroutines do que gargalos comprovados.
- Mais configuração do que casos de uso.
- Mais código de infraestrutura do que código de negócio.

Se qualquer item for verdadeiro, simplifique primeiro.

---

## 11) A stack fechada

Apenas estas dependências são permitidas. **Nunca adicione uma dependência fora
desta lista.**

| Funcionalidade | Permitido | Proibido |
|---|---|---|
| MTProto | `github.com/gotd/td` | GoTGProto ou qualquer outro wrapper |
| Banco de dados | `turso.tech/database/tursogo` (driver `turso`) | Drivers SQLite, `mattn`, `modernc.org/sqlite`, GORM, qualquer ORM |
| CLI/config | `github.com/spf13/cobra`, `github.com/spf13/viper` | — |
| Terminal | `golang.org/x/term` (entrada mascarada no assistente/auth) | — |
| Logging | stdlib `log/slog` (por trás de `logger.Logger`) | zerolog, zap, logrus |
| HTTP | stdlib `net/http` (apenas para o dashboard) | chi, gin, echo, fiber |
| Testes de propriedade | `pgregory.net/rapid` (apenas arquivos `_test.go`) | — |

---

## 12) Regras rígidas de Go

### 12.1 Contexto

`context.Context` é o primeiro argumento de toda operação de I/O, rede, banco ou
cancelamento.

### 12.2 Erros

- Use wrapping com contexto suficiente para depuração.
- Erros atravessando camadas devem preservar causa e origem via `errors.Wrap(layer, op, err)`.
- Formato: `"layer: op: cause"`.
- Sentinels residem em `internal/errors/errors.go`.
- Não use `panic()` em código de produção.
- `recover()` só pode existir no limite da goroutine do dispatcher.

### 12.3 Estado global

- Sem `init()` para comportamento de produção.
- Sem estado mutável global em `internal/`.
- Sem side effects escondidos na importação.

### 12.4 Interfaces e abstrações

- Não crie interface sem necessidade real.
- Não crie camadas "flexíveis" por hipótese futura.
- Não crie generics/abstrações quando uma função simples resolve.

### 12.5 Testes

- Toda mudança relevante precisa de teste.
- Bug corrigido deve ter teste reproduzindo o erro.
- Refatoração deve preservar o comportamento verificado por teste.

---

## 13) Regras rígidas de banco de dados

### 13.1 O driver permitido

- Banco local: `turso.tech/database/tursogo`
- Driver registrado: `turso`

### 13.2 Proibições

Não adicionar: drivers SQLite tradicionais, `mattn/go-sqlite3`, `modernc.org/sqlite`,
GORM, ORM genérico, libs "compatíveis com SQLite" por comodidade.

### 13.3 Regra de compatibilidade

Não assumir comportamento de SQLite tradicional sem validar primeiro se ele existe
no Turso Database na versão usada.

### 13.4 Regra de banco compartilhado

É proibido assumir que múltiplos processos podem abrir simultaneamente o mesmo
arquivo `.db`.

Antes de propor qualquer arquitetura multi-processo:

1. Consultar `docs/specs/TURSOGO.md`
2. Consultar skill `turso-db`
3. Verificar documentação oficial da versão utilizada
4. Verificar DSN efetivamente configurado

Sem essas quatro verificações, a proposta é inválida.

### 13.5 Regra para `experimental=multiprocess_wal`

Só considerar essa opção se houver:

- Documentação oficial relevante confirmando suporte na versão em uso
- Aprovação explícita no desenho arquitetural (ADR)
- Justificativa clara do risco
- Testes comprovando funcionamento

Se não houver isso, não proponha múltiplos processos compartilhando o mesmo arquivo.

### 13.6 Recurso nativo antes de solução manual

Antes de implementar manualmente qualquer um destes itens, verifique se o Turso
Database já oferece suporte nativo:

- CDC
- MVCC
- Encryption at rest
- Sync / replication
- FTS
- Vector search
- WAL / modo de journaling
- Busy timeout
- Change feed

Se existir suporte nativo e for adequado, prefira o suporte nativo.

### 13.7 SQL

- Todo o SQL reside em `internal/storage/repository.go`.
- Placeholders são apenas `?`.
- Escritas recorrentes usam prepared statements.
- Escrever SQL espalhado por outras camadas é proibido.

---

## 14) Regras específicas para o limiar-collector

### 14.1 Escrita no banco

- Apenas uma goroutine deve escrever no banco.
- Essa goroutine é o `Collector.dbWriter`.
- Nenhuma outra goroutine escreve diretamente.
- Leitura pelo dashboard é read-only.

### 14.2 Telegram

- Tipos do gotd/td não vazam para fora de `internal/telegram`.
- Camadas externas usam a facade `TelegramClient` e modelos de domínio.
- Updates deixam o pacote telegram como `[]byte` JSON em `telegram.Update`.

### 14.3 Dashboard

- Dashboard é leitura + SSE.
- Dashboard nunca escreve no banco.
- Dashboard nunca importa `internal/telegram`.

### 14.4 Logging

- Logging concreto é criado apenas em `internal/logger/slog.go`.
- Outras camadas recebem `logger.Logger` por injeção.
- Chaves sensíveis (`api_hash`, `session`, `token`, `password`, `auth_code`) são mascaradas.

### 14.5 Composição

- Dependências concretas são construídas apenas em `cmd/limiar-collector/main.go`.
- A CLI depende da interface `cli.Provider`.
- DI manual — sem framework.

---

## 15) Regras específicas para o limiar-processor

### 15.1 Regras do processor

1. **`raw_messages` são read-only.** O processor nunca escreve em `raw_messages`.
2. **Mesmo banco, tabelas separadas.** Tabelas do collector são intocáveis.
3. **Sem dependência do Telegram.** `internal/processor` nunca importa `internal/telegram`.
4. **Preços são INTEGER (centavos).** Nunca usar `float64` para valor monetário.
5. **Processamento idempotente.** `ON CONFLICT DO NOTHING` em `(channel_id, message_id)`.
6. **Mesma closed stack.** Nenhuma dependência nova além do que este arquivo permite.

### 15.2 Arquitetura do processor

Antes de propor goroutines, workers, canais ou paralelismo, verificar:

- Se o banco é o gargalo real.
- Se o polling existe por limitação técnica ou por simplicidade deliberada.
- Se há recurso nativo do Turso que elimina o polling.

### 15.3 Quando pensar em CDC, MVCC, sync ou multiprocess

Se a proposta tocar em: feed de mudanças, processar mudanças em tempo real,
sincronização local/remota, multi-process access, escrita concorrente — então a
skill `turso-db` deve ser consultada novamente antes de codar.

---

## 16) Regras para uso de IA

Nenhuma decisão de negócio pode depender exclusivamente de saída de LLM.

Toda inferência de IA deve possuir:

- Confidence score
- Rastreabilidade (qual input gerou qual output)
- Possibilidade de reprocessamento

Prompts fazem parte da lógica do sistema e devem ser versionados.

LLMs não podem ser chamados por:

- Collector (Fase 1)
- Processor (Fase 2)

Apenas a camada de IA (Fase 3) pode invocar modelos de linguagem.

---

## 17) Regras de concorrência

### 17.1 Simplicidade primeiro

- Não criar goroutine por hábito.
- Não criar worker pool por reflexo.
- Não criar channels "porque Go gosta disso".
- Não paralelizar sem gargalo real comprovado.

### 17.2 Verificação antes da concorrência

Antes de adicionar concorrência, responder:

1. O que está bloqueando de verdade?
2. CPU, I/O, banco, rede ou mutex?
3. Existe benchmark ou evidência?
4. Qual parte realmente se beneficia da paralelização?

### 17.3 Segurança

- Evite races.
- Evite acesso concorrente a mapas, slices compartilhados e handles de banco.
- Se houver concorrência, ela deve ser visível e testável.

---

## 18) Regras de segurança

- Nunca logar segredos, tokens, sessão, `api_hash`, `password`, `auth_code` ou valores equivalentes.
- Não persistir segredo em texto claro sem justificativa.
- Não usar permissões de arquivo frouxas para dados sensíveis.
- Não assumir que filesystem seguro substitui proteção de conteúdo.
- Se o banco puder ser protegido por criptografia nativa e isso fizer sentido,
  considere isso antes de criar soluções caseiras.

---

## 19) Mudanças arquiteturais

Qualquer alteração que modifique:

- Modelo de concorrência
- Modelo de dados
- Limites entre fases
- Estratégia de armazenamento
- Dependências permitidas
- Comunicação entre componentes

Exige:

1. ADR em `docs/adr/`
2. Atualização da documentação relevante
3. Justificativa explícita

Sem os três, a mudança não pode ser feita.

---

## 20) Regras de documentação

### 20.1 Se mudar comportamento, atualize documentação

Se a mudança alterar: arquitetura, contratos de camadas, fluxo de dados,
dependências permitidas, restrições do banco, concorrência, configuração —
atualize a documentação no mesmo PR/commit.

### 20.2 Nunca deixar documentação e código divergirem sem nota

Se o código mudou e a documentação ainda não, o agente deve deixar isso explícito.

---

## 21) Critérios de qualidade

```sh
go build ./...        # saída 0
go vet ./...          # zero problemas
go test ./...         # todos passam
go test -race ./...   # sem data races
```

O build não deve conter nenhum import de `sqlite`, `mattn` ou `gorm`.

**Critério mínimo de verificação:** Toda mudança precisa de uma forma objetiva de
ser validada (teste, build, inspeção, benchmark, evidência documental). Sem
verificação, a tarefa não está concluída.

---

## 22) Processo esperado para qualquer tarefa

### Antes de editar

1. Ler `docs/` relevante.
2. Consultar a skill correta.
3. Declarar premissas.
4. Identificar ambiguidade.
5. Definir sucesso verificável.

### Durante a edição

- Fazer a menor mudança possível.
- Não mexer no que não foi pedido.
- Preservar estilo existente.
- Remover apenas o que a própria mudança tornou inútil.

### Depois de editar

- Verificar build/testes.
- Conferir impacto colateral.
- Atualizar docs se necessário.
- Reportar limitações ou pendências com honestidade.

---

## 23) Regra final

Se o problema parecer simples, trate como simples. Se parecer complexo, prove que
é complexo antes de complicar a solução.

Não assumir. Não inventar. Não reescrever o projeto inteiro porque a intuição do
modelo achou elegante.

Use a documentação. Use as skills. Use a menor solução que funciona.

---

## Referência rápida — Estrutura do projeto

```
cmd/limiar-collector/main.go      — raiz de composição do collector
cmd/limiar-processor/main.go      — raiz de composição do processor
internal/
├── cli/            — Comandos Cobra (root, auth, channels, run, dashboard)
├── collector/      — Orquestração: Collector, MessageHandler, Classifier
├── config/         — Carga (Viper + .env + wizard) + Validação
├── dashboard/      — Servidor HTTP (net/http) + Broker SSE
├── errors/         — Sentinels + ajudante Wrap
├── logger/         — Interface Logger, SlogLogger, PrettyHandler, NopLogger
├── processor/      — Normalização, classificação, persistência (Fase 2)
├── storage/        — Abertura/fechamento do BD, migrações, Repository (todo SQL)
└── telegram/       — Facade TelegramClient, Dispatcher, PeerStore,
                      TursoSessionStorage, codificação/extração
tools/
└── payload-analyzer/  — Análise offline de payload (dev-time)
docs/
├── ARCHITECTURE.md
├── CONTEXT.md
├── PRODUCT_BRIEF.md
├── adr/
├── guidelines/
└── specs/
```

---

## Referência rápida — Configuração

Todas as variáveis são prefixadas com `LIMIAR_` e lidas de env + `.env` opcional.

| Variável | Padrão | Valores válidos |
|---|---|---|
| `LIMIAR_APP_ID` | — (obrigatório) | inteiro > 0 |
| `LIMIAR_API_HASH` | — (obrigatório) | não vazio (mascarado nos logs) |
| `LIMIAR_DB_PATH` | `./limiar.db` | qualquer caminho válido |
| `LIMIAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `pretty` | `json`, `text`, `pretty` |
| `LIMIAR_SHUTDOWN_TIMEOUT` | `15` | 1–300 (segundos) |
| `LIMIAR_MAX_RETRIES` | `10` | inteiro positivo |
| `LIMIAR_IO_TIMEOUT` | `30s` | Duração Go |
| `LIMIAR_DISPATCHER_BUFFER_SIZE` | `256` | 64–4096 |
| `LIMIAR_DB_WRITER_BUFFER_SIZE` | `512` | 128–8192 |
| `LIMIAR_HISTORY_MAX` | `5000` | 100–100000 |
| `LIMIAR_HISTORY_MAX_DAYS` | `30` | 1–365 |
| `LIMIAR_PROCESSOR_POLL_INTERVAL` | `5s` | [1s, 5m] |
| `LIMIAR_PROCESSOR_BATCH_SIZE` | `50` | 1–1000 |

---

## Referência rápida — Notas de estilo

- Mensagens de log usam prefixos de emoji: 📡 📩 📜 🔄 ❌ ✅ 🛑 ⏰ 🌐 ⚠️
- Strings visíveis ao usuário estão em Português (pt-BR).
- Comentários de código e documentação estão em Português (pt-BR).
- Mensagens de erro seguem o formato `"layer: op: cause"`.
