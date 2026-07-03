# ADR 009 — Orquestrador Unificado em Processo Único

## Status

Aceito

## Contexto

A arquitetura inicial previa que `limiar-collector`, `limiar-processor` e os dashboards fossem executados como processos independentes do sistema operacional. Embora isso ofereça desacoplamento forte, na prática de deploy (especialmente para MVP), gerenciar múltiplos processos traz complexidade operacional indesejada (setup de systemd, docker-compose com dependências de startup, concorrência no arquivo SQLite local).

## Decisão

Adotar uma arquitetura de orquestrador unificado (`cmd/limiar`). O comando `limiar run` inicia o collector, o processor e o dashboard no **mesmo processo do sistema operacional**, coordenando o ciclo de vida (startup e graceful shutdown) usando a biblioteca `golang.org/x/sync/errgroup`.

Eles ainda mantêm fronteiras rígidas de código (`internal/collector` não importa `internal/processor`) e comunicam-se exclusivamente através do banco de dados Tursogo (`limiar.db`), mas agora compartilham o mesmo `*sql.DB` com `SetMaxOpenConns(1)`, mitigando contenção e múltiplos handles físicos sobre o arquivo `.db`.

## Consequências

- **Vantagem:** Deploy simplificado (um binário sobe tudo).
- **Vantagem:** Gerenciamento centralizado da conexão Tursogo, com apenas uma conexão física ativa por processo, evitando locks/corrupção por acesso local concorrente ao mesmo arquivo.
- **Vantagem:** Desligamento coordenado (se um falha, o `errgroup` derruba todos).
- **Desvantagem:** Vazamento de memória ou pânico irrecuperável em um componente derruba todo o sistema (aceitável dado o mecanismo de recover existente).
- **Nota Histórica:** Os comandos `limiar-collector run` e `limiar-processor` standalone ainda existem para fins de debug e desenvolvimento.
