# limiar-collector

Fase 1 do **Limiar**, um pipeline que monitora canais promocionais brasileiros do Telegram através de um userbot MTProto e persiste as mensagens **brutas (raw)** como JSON para processamento posterior.

O `limiar-collector` faz apenas um trabalho: conectar-se ao Telegram, autenticar-se como um userbot, gerenciar a lista de canais monitorados e capturar os payloads brutos das mensagens em um banco de dados embutido. Há **zero** processamento nesta fase — sem normalização, classificação, enriquecimento ou desduplicação além da persistência segura. O objetivo da Fase 1 é descobrir o formato real dos dados do Telegram para que as fases posteriores (`limiar-processor`, `limiar-api`) possam ser construídas sobre ele.

## O que ele é

- Um binário Go standalone (`cmd/limiar-collector`, módulo `github.com/limiar/collector`).
- Um userbot MTProto construído diretamente sobre o [`gotd/td`](https://github.com/gotd/td) (sem wrappers de terceiros).
- Um coletor de mensagens brutas que grava payloads JSON em um banco de dados embutido [Tursogo](https://turso.tech) (driver `turso` via `database/sql`, **sem CGO**).

## O que ele NÃO é

- Não é um bot do Telegram (ele não responde a mensagens nem interage com usuários).
- Não é um scraper HTTP (ele fala nativamente o protocolo MTProto).
- Não é um sistema de alertas (ele é uma etapa de um pipeline de dados).
- Não é um processador ou API — normalização, classificação, desduplicação, chamadas de LLM, REST e SSE pertencem todos a fases futuras e estão explicitamente fora do escopo.

## Não requer CGO

O Tursogo utiliza `purego` para FFI, portanto o binário é compilado e executado sem uma toolchain C e sem `CGO_ENABLED=1`. Sem driver SQLite, sem GORM, sem ORM.

## Instalação

```sh
go build ./...
# ou compile apenas o binário:
go build -o limiar-collector ./cmd/limiar-collector
```

## Comandos

O binário expõe três subcomandos. `auth` e `channels` fazem logging no formato **texto** (interativo); `run` faz logging no formato **JSON** (serviço de produção).

### `auth` — autenticar uma vez e persistir a sessão

Interativo, idempotente. Solicita o número de telefone, depois o código de login e, em seguida, a senha de 2FA se a conta tiver a autenticação de dois fatores ativada. A sessão é persistida na tabela `sessions` (linha única). Executar `auth` novamente quando já existe uma sessão válida não tem efeito.

```sh
LIMIAR_APP_ID=12345 LIMIAR_API_HASH=abcdef... ./limiar-collector auth
# Número de telefone (formato internacional, ex: +5511999999999): +55...
# Código de login: 12345
# Senha 2FA: ****        (apenas se a autenticação de dois fatores estiver ativada)
```

### `channels` — gerenciar a lista de canais monitorados

Requer uma sessão válida (caso contrário, retorna `ErrNotAuthenticated`).

```sh
./limiar-collector channels list
./limiar-collector channels add <username>      # resolve @username via MTProto, persiste o canal
./limiar-collector channels remove <username>   # ErrChannelNotFound se ausente
```

### `run` — iniciar o serviço coletor

Não interativo, pronto para produção. Requer uma sessão válida. Faz o backfill das 20 mensagens mais recentes na primeira execução de um canal, depois captura as mensagens ao vivo e persiste cada payload JSON bruto. Desliga de forma graciosa (graceful shutdown) ao receber `SIGTERM`/`SIGINT`, finalizando as gravações pendentes dentro de `LIMIAR_SHUTDOWN_TIMEOUT` segundos.

```sh
LIMIAR_APP_ID=12345 LIMIAR_API_HASH=abcdef... ./limiar-collector run
```

## Configuração (variáveis de ambiente)

Toda a configuração é lida de variáveis de ambiente com o prefixo `LIMIAR_` usando o Viper. `Load()` preenche a estrutura e aplica os padrões; `Validate()` é chamado explicitamente antes de qualquer operação de E/S e reporta todos os campos inválidos de uma vez.

| Variável | Campo | Padrão | Intervalo válido / valores |
|----------|-------|---------|----------------------|
| `LIMIAR_APP_ID` | `AppID` | — (obrigatório) | inteiro diferente de zero |
| `LIMIAR_API_HASH` | `APIHash` | — (obrigatório) | string não vazia (mascarada nos logs) |
| `LIMIAR_DB_PATH` | `DBPath` | `./limiar.db` | qualquer caminho válido |
| `LIMIAR_LOG_LEVEL` | `LogLevel` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `LogFormat` | `json` | `json`, `text` |
| `LIMIAR_SHUTDOWN_TIMEOUT` | `ShutdownTimeout` | `15` | 1–300 (segundos) |
| `LIMIAR_MAX_RETRIES` | `MaxRetries` | `10` | inteiro positivo |
| `LIMIAR_IO_TIMEOUT` | `IOTimeout` | `30s` | Duração do Go |
| `LIMIAR_DISPATCHER_BUFFER_SIZE` | `DispatcherBufferSize` | `256` | 64–4096 |
| `LIMIAR_DB_WRITER_BUFFER_SIZE` | `DBWriterBufferSize` | `512` | 128–8192 |

`AppID` e `APIHash` são obrigatórios e nunca possuem valor padrão; um valor ausente para qualquer um deles produz um erro de validação. O valor de `APIHash` é mascarado por `Config.String()` e omitido de todas as saídas de log.

## Stack

| Funcionalidade | Tecnologia |
|---------|-----------|
| MTProto | `gotd/td` (sem wrapper) |
| Banco de dados | `turso.tech/database/tursogo` (driver `turso`, sem CGO) |
| CLI / configuração | `cobra` + `viper` |
| Logging | `log/slog` por trás de uma interface `Logger` |
| Testes de propriedade | `pgregory.net/rapid` (apenas arquivos de teste) |

## Documentação

- `docs/CONTEXT.md` — visão geral do sistema, escopo, roadmap de fases
- `docs/ARCHITECTURE.md` — diagrama do pipeline, modelo de concorrência, modelo de dados
- `docs/specs/GOTD-TD.md`, `docs/specs/TURSOGO.md` — especificações de uso de bibliotecas
- `docs/guidelines/` — diretrizes de extensão para armazenamento, telegram, collector
- `docs/adr/` — registros de decisão de arquitetura (ADRs)
- `AGENTS.md` — regras para agentes de IA e contribuidores
