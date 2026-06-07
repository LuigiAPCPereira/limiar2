# Requirements Document

## Introduction

`limiar-collector` é o primeiro estágio do pipeline Limiar (`limiar-collector → limiar-processor → limiar-api → Frontend`). Trata-se de um binário Go independente que se conecta ao Telegram via protocolo MTProto como userbot, monitora um conjunto configurado de canais promocionais brasileiros no Telegram e persiste os payloads **brutos** das mensagens como JSON em um banco de dados embarcado Tursogo.

Esta especificação cobre **apenas a Fase 1**. O único objetivo da Fase 1 é capturar a forma real dos dados do Telegram: há **zero processamento, normalização, enriquecimento, deduplicação (além da persistência segura) ou classificação semântica**. Essas responsabilidades pertencem ao `limiar-processor` e ao `limiar-api` e estão explicitamente fora do escopo.

O binário expõe três subcomandos CLI:

- `auth` — autenticação interativa, única vez, idempotente que persiste uma sessão Telegram (logger em texto).
- `channels` — gerenciamento da lista de canais monitorados: `list`, `add <username>`, `remove <username>` (requer autenticação prévia, logger em texto).
- `run` — serviço coletor não-interativo, pronto para produção, que monitora canais concorrentemente e persiste mensagens brutas, com logging estruturado em JSON e shutdown gracioso em `SIGTERM`/`SIGINT`.

A arquitetura exige padrões de design específicos (Facade, Observer, Strategy, Adapter), injeção de dependências manual, regras estritas de concorrência, políticas de resiliência e um stack tecnológico fechado. Essas restrições são capturadas como requisitos testáveis abaixo.

## Glossary

- **Collector**: O binário `limiar-collector` como um todo, composto pelas camadas CLI, Telegram, Collector, Storage, Config, Logger e Errors.
- **CLI**: A camada de interface de linha de comando construída com Cobra e Viper, expondo os subcomandos `auth`, `channels` e `run`. A construção de dependências concretas ocorre apenas em `cmd/limiar-collector/main.go`.
- **TelegramClient**: A interface Facade que esconde toda a complexidade do gotd/td (MTProto). Expõe `Connect`, `Auth`, `IsAuthenticated`, `AddUpdateHandler`, `ResolveChannel` e `Run`.
- **Dispatcher**: O componente Observer que recebe atualizações do Telegram e as distribui para instâncias registradas de `UpdateHandler` via canais Go bufferizados, invocando handlers em goroutines.
- **UpdateHandler**: Uma interface registrada no Dispatcher para receber atualizações do Telegram.
- **MessageHandler**: A implementação de `UpdateHandler` da Fase 1 que adapta uma atualização bruta do gotd/td em um `RawMessage` e o encaminha para persistência. É stateless e seguro para invocação concorrente.
- **Classifier**: A interface Strategy para classificação de mensagens, plugável desde o início.
- **NoopClassifier**: A implementação de `Classifier` da Fase 1 que retorna sua entrada sem modificação (pass-through).
- **DBWriter**: A goroutine dedicada única que serializa todas as escritas no banco de dados via fan-in. É o único escritor no `*sql.DB`.
- **PeerStore**: Um `map[int64]*Peer` em memória protegido por `sync.RWMutex`, persistido via repositório Tursogo.
- **Repository**: O componente de armazenamento em `internal/storage/repository.go` que centraliza todas as queries SQL contra o Tursogo.
- **Config**: O objeto de configuração carregado a partir de variáveis de ambiente prefixadas com `LIMIAR_`, com funções separadas `Load()` e `Validate()`.
- **Logger**: A interface de logging; a implementação padrão encapsula o stdlib `slog` e é instanciada apenas em `internal/logger/slog.go`.
- **Tursogo**: O driver de banco de dados embarcado `turso.tech/database/tursogo` (nome do driver `turso`) usado através de `database/sql`. Sem CGO, sem driver SQLite, sem GORM, sem ORM.
- **RawMessage**: Modelo interno `{ID, ChannelID, MessageID, Payload []byte, ReceivedAt, SchemaVersion}` representando um payload de mensagem Telegram capturado.
- **Channel**: Modelo interno `{ID, Username, Title, Active, AddedAt, LastMessageID, LastCollectedAt}` representando um canal monitorado.
- **Peer**: Modelo interno `{ID, AccessHash, Type, Username, UpdatedAt}` representando um peer Telegram.
- **Session**: Estado de autenticação Telegram persistido armazenado na tabela `sessions`.
- **Flood_Wait**: Um sinal de rate-limit do Telegram instruindo o cliente a esperar uma duração específica antes de tentar novamente.
- **ShutdownTimeout**: A duração máxima configurável (1-300 segundos, padrão 15) permitida para completar o shutdown gracioso.

## Requirements

### Requisito 1: Autenticar como Userbot e Persistir Sessão

**História de Usuário:** Como operador, quero autenticar o coletor com o Telegram uma vez e ter a sessão salva, para que o coletor possa reconectar posteriormente sem reinserir credenciais.

#### Critérios de Aceitação

1. WHEN o operador executa o subcomando `auth`, THE CLI SHALL usar um Logger em formato texto.
2. WHEN o subcomando `auth` é invocado, THE TelegramClient SHALL solicitar o número de telefone, depois o código de login, nessa ordem.
3. IF o Telegram exigir autenticação de dois fatores durante o fluxo de `auth`, THEN THE TelegramClient SHALL solicitar a senha 2FA e capturar a condição `ErrPasswordRequired`.
4. WHEN a autenticação é concluída com sucesso, THE Repository SHALL persistir a Session na tabela `sessions` de modo que `SELECT COUNT(*) FROM sessions` retorne 1.
5. WHEN o subcomando `auth` é invocado enquanto já existe uma Session válida, THE TelegramClient SHALL concluir sem criar uma Session duplicada, mantendo `SELECT COUNT(*) FROM sessions` igual a 1.
6. IF a Session armazenada estiver corrompida, THEN THE TelegramClient SHALL retornar um erro que satisfaça `errors.Is(err, ErrSessionCorrupted)`.

### Requisito 2: Gerenciar a Lista de Canais Monitorados

**História de Usuário:** Como operador, quero listar, adicionar e remover canais monitorados, para que eu possa controlar quais canais Telegram o coletor observa.

#### Critérios de Aceitação

1. WHEN o operador executa qualquer subcomando de `channels`, THE CLI SHALL usar um Logger em formato texto.
2. IF o operador executar um subcomando de `channels` enquanto nenhuma Session válida existir, THEN THE CLI SHALL retornar um erro que satisfaça `errors.Is(err, ErrNotAuthenticated)`.
3. WHEN o operador executa `channels add <username>`, THE Repository SHALL inserir um registro Channel com o username informado e definir seu campo `active` como true.
4. WHEN o operador executa `channels list`, THE CLI SHALL exibir todo Channel armazenado na tabela `channels`.
5. WHEN o operador executa `channels remove <username>` para um Channel existente, THE Repository SHALL remover aquele Channel de modo que um `channels list` subsequente não inclua o username removido.
6. IF o operador executar `channels remove <username>` para um username que não está armazenado, THEN THE CLI SHALL retornar um erro que satisfaça `errors.Is(err, ErrChannelNotFound)`.

### Requisito 3: Executar o Serviço Coletor e Persistir Mensagens Brutas

**História de Usuário:** Como operador, quero executar o coletor como serviço que captura mensagens recebidas dos canais, para que os payloads brutos do Telegram sejam salvos para processamento posterior.

#### Critérios de Aceitação

1. WHEN o operador executa o subcomando `run`, THE Collector SHALL usar um Logger em formato JSON.
2. IF o operador executar o subcomando `run` enquanto nenhuma Session válida existir, THEN THE Collector SHALL retornar um erro que satisfaça `errors.Is(err, ErrNotAuthenticated)`.
3. WHEN uma mensagem é recebida de um Channel monitorado, THE MessageHandler SHALL adaptar a atualização do gotd/td em um RawMessage contendo o payload JSON serializado.
4. WHEN um RawMessage é capturado, THE DBWriter SHALL persisti-lo na tabela `raw_messages` de modo que `SELECT payload FROM raw_messages` retorne JSON válido.
5. WHEN um RawMessage de um Channel é persistido, THE Repository SHALL atualizar os campos `last_message_id` e `last_collected_at` daquele Channel.
6. WHEN o Collector inicia e `last_message_id` de um Channel é maior que zero, THE Collector SHALL retomar o monitoramento a partir desse ID para evitar reprocessamento.
7. IF `last_message_id` for zero (primeira execução), THEN THE Collector SHALL buscar as 20 mensagens mais recentes do histórico do canal, persistir seus payloads brutos e definir `last_message_id` com o ID da mensagem mais recente obtida antes de iniciar o monitoramento de novas mensagens.
8. THE MessageHandler SHALL persistir o payload bruto sem enriquecê-lo, classificá-lo ou deduplicá-lo além da persistência segura.
9. WHEN um RawMessage passa pela classificação, THE NoopClassifier SHALL retornar o RawMessage sem modificação.
10. IF uma mensagem não puder ser persistida, THEN THE Collector SHALL emitir uma entrada de log identificando a mensagem perdida em vez de descartá-la silenciosamente.

### Requisito 4: Shutdown Gracioso

**História de Usuário:** Como operador, quero que o coletor em execução encerre de forma limpa quando parado, para que mensagens em trânsito sejam drenadas e o banco de dados seja fechado com segurança.

#### Critérios de Aceitação

1. WHEN o processo do Collector recebe `SIGTERM` ou `SIGINT`, THE Collector SHALL iniciar o shutdown gracioso cancelando o contexto raiz.
2. WHEN o shutdown gracioso começa, THE Collector SHALL aguardar em um `sync.WaitGroup` até que todas as goroutines de monitoramento de canais tenham sido drenadas antes de fechar a conexão com o banco de dados.
3. WHEN o shutdown gracioso é acionado, THE Collector SHALL completar o shutdown e encerrar dentro do ShutdownTimeout configurado.
4. WHEN o contexto raiz é cancelado, THE Collector SHALL propagar o cancelamento para todas as goroutines.

### Requisito 5: Monitoramento Concorrente de Canais

**História de Usuário:** Como operador, quero que os canais sejam monitorados concorrentemente, para que mensagens de todos os canais sejam capturadas sem que um canal bloqueie outro.

#### Critérios de Aceitação

1. WHEN o Collector monitora múltiplos Channels, THE Collector SHALL executar cada Channel em sua própria goroutine em vez de processar Channels sequencialmente.
2. THE Dispatcher SHALL distribuir atualizações para UpdateHandlers registrados através de canais Go bufferizados com um tamanho de buffer por canal de pelo menos 256 entradas.
3. THE Dispatcher SHALL invocar cada UpdateHandler registrado em uma goroutine.
4. THE MessageHandler SHALL ser seguro para invocação concorrente sem adquirir locks.
5. THE DBWriter SHALL ser o único componente que escreve na conexão `*sql.DB`, recebendo valores RawMessage de produtores através de fan-in.
6. THE PeerStore SHALL proteger seu mapa de peers em memória com um `sync.RWMutex`.

### Requisito 6: Resiliência de Conexão

**História de Usuário:** Como operador, quero que o coletor se recupere de falhas de conexão e rate limits, para que continue coletando sem intervenção manual.

#### Critérios de Aceitação

1. IF a conexão com o Telegram for perdida, THEN THE TelegramClient SHALL tentar reconectar usando backoff exponencial com delay base de 1 segundo, multiplicador de 2, 10% de jitter e teto de 5 minutos.
2. THE TelegramClient SHALL interromper tentativas de reconexão após atingir a contagem configurada de MaxRetries.
3. WHEN o Telegram sinaliza um Flood_Wait, THE TelegramClient SHALL aguardar exatamente a duração especificada antes de tentar novamente.
4. IF uma escrita no banco de dados falhar, THEN THE DBWriter SHALL tentar novamente até a contagem de retries configurada, e em falha persistente SHALL retornar um erro que satisfaça `errors.Is(err, ErrDBWriteFailed)`.
5. THE Collector SHALL aplicar o `IOTimeout` configurado a todas as operações de entrada/saída.

### Requisito 7: Carregamento e Validação de Configuração

**História de Usuário:** Como operador, quero que a configuração seja carregada de variáveis de ambiente e validada antes do uso, para que erros de configuração sejam reportados claramente antes de qualquer I/O.

#### Critérios de Aceitação

1. THE Config SHALL ler todas as configurações a partir de variáveis de ambiente prefixadas com `LIMIAR_`.
2. THE Config SHALL expor `Load()` e `Validate()` como funções separadas.
3. THE Collector SHALL chamar `Validate()` explicitamente antes de realizar qualquer operação de entrada/saída.
4. WHEN `Validate()` é executado, THE Config SHALL coletar e reportar todos os erros de validação juntos em vez de parar no primeiro erro.
5. IF `AppID` ou `APIHash` estiver ausente, THEN `Validate()` SHALL reportar um erro de validação para cada campo obrigatório faltante.
6. THE Config SHALL aplicar valores padrão: `DBPath` = `./limiar.db`, `LogLevel` = `info`, `LogFormat` = `json`, `ShutdownTimeout` = 15.
7. THE Config SHALL aceitar valores de `LogLevel` como `debug`, `info`, `warn` ou `error`, e valores de `LogFormat` como `json` ou `text`.
8. THE Config SHALL aceitar valores de `ShutdownTimeout` de 1 a 300, valores de `DispatcherBufferSize` de 64 a 4096 e valores de `DBWriterBufferSize` de 128 a 8192.
9. THE `Config.String()` method SHALL mascarar o valor de `APIHash` e quaisquer dados de sessão.
10. THE Logger SHALL excluir `APIHash` e dados de sessão de toda saída de log.

### Requisito 8: Domínio de Erros e Tratamento de Falhas

**História de Usuário:** Como desenvolvedor, quero um domínio de erros consistente com sentinelas nomeados, para que chamadores possam raciocinar sobre falhas programaticamente em vez de comparar strings.

#### Critérios de Aceitação

1. THE Collector SHALL definir os erros sentinela `ErrNotAuthenticated`, `ErrChannelNotFound`, `ErrSessionCorrupted` e `ErrDBWriteFailed` em `internal/errors/errors.go`.
2. WHEN um erro cruza um limite de camada, THE Collector SHALL envolver o erro com contexto identificando a camada de origem.
3. THE Collector SHALL permitir que chamadores identifiquem categorias de erro usando `errors.Is` contra os sentinelas definidos.
4. THE código de produção SHALL excluir chamadas a `panic()`.
5. WHERE `recover()` é usado, THE Collector SHALL colocá-lo apenas nos limites de goroutines do Dispatcher e SHALL logar toda condição recuperada.

### Requisito 9: Restrições da Camada de Armazenamento

**História de Usuário:** Como desenvolvedor, quero toda persistência centralizada e usando apenas o driver aprovado, para que a camada de armazenamento permaneça consistente e livre de riscos de concorrência.

#### Critérios de Aceitação

1. THE Storage layer SHALL usar apenas o driver Tursogo e SHALL excluir qualquer driver SQLite, GORM ou outro ORM.
2. THE Repository SHALL centralizar todas as queries SQL em `internal/storage/repository.go`.
3. THE Repository SHALL usar apenas `?` como token de placeholder SQL.
4. THE Repository SHALL usar prepared statements para queries recorrentes.
5. THE Storage layer SHALL criar o schema definido em `migrations/001_initial.sql` contendo as tabelas `sessions`, `peers`, `channels` e `raw_messages`.
6. THE Collector SHALL excluir escritas ao `*sql.DB` de qualquer componente que não seja o DBWriter.

### Requisito 10: Arquitetura, Padrões e Wiring

**História de Usuário:** Como desenvolvedor, quero que os padrões de design prescritos e o wiring de dependências sejam aplicados, para que o codebase permaneça modular e desacoplado dos internos do gotd/td.

#### Critérios de Aceitação

1. THE TelegramClient interface SHALL esconder todos os tipos do gotd/td de modo que as camadas CLI e Collector não importem gotd/td.
2. THE Collector SHALL realizar construção de dependências concretas apenas em `cmd/limiar-collector/main.go`, usando injeção de dependências manual sem framework de DI, variáveis globais ou funções `init()`.
3. THE pacotes internos de CLI SHALL não construir dependências concretas de storage ou telegram.
4. THE Adapter que converte uma atualização bruta do gotd/td em um RawMessage SHALL residir em `collector/handler.go`.
5. THE Classifier SHALL ser definido como uma interface plugável, com NoopClassifier fornecido como a implementação da Fase 1.
6. THE Logger SHALL ser instanciado apenas em `internal/logger/slog.go`.
7. THE pacotes internos SHALL excluir funções `init()` e variáveis de estado globais.

### Requisito 11: Quality Gates de Build, Vet e Testes

**História de Usuário:** Como desenvolvedor, quero que o projeto passe nos quality gates padrão do Go, para que o codebase permaneça compilável, verificado e livre de race conditions.

#### Critérios de Aceitação

1. WHEN `go build ./...` é executado, THE Collector SHALL compilar com código de saída 0.
2. WHEN `go vet ./...` é executado, THE Collector SHALL reportar zero problemas e encerrar com código de saída 0.
3. WHEN `go test ./...` é executado, THE Collector SHALL passar em todos os testes com código de saída 0.
4. WHEN `go test -race ./...` é executado, THE Collector SHALL passar em todos os testes sem data races detectados e encerrar com código de saída 0.
5. THE Collector SHALL excluir importações dos pacotes `sqlite`, `mattn` e `gorm`.

## Fora do Escopo (Fase 1)

Os itens a seguir **não** são requeridos para esta especificação e não devem ser implementados na Fase 1:

- API HTTP, endpoints, health checks, métricas, SSE ou WebSocket (chi está reservado apenas para uso futuro).
- Normalização, enriquecimento ou classificação semântica de mensagens.
- Chamadas a LLM.
- Deduplicação além da persistência segura.
- Qualquer responsabilidade pertencente ao `limiar-processor` ou `limiar-api`.
