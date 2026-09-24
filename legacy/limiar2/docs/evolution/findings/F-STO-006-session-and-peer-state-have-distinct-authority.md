# F-STO-006 — Sessão MTProto e peer cache possuem autoridades distintas

Authority: Non-authoritative Finding
Status: Open

## Descoberta

A implementação legada co-localiza `sessions` e `peers` na mesma persistência implementada
via Tursogo, mas o comportamento observado mostra que esses estados possuem finalidade,
criticidade e ciclo de vida diferentes. Portanto, a coexistência física atual não sustenta
tratá-los como uma única autoridade futura nem copiar mecanicamente as duas tabelas para o
novo storage.

### Sessão MTProto

O registry de transição em `docs/adr/README.md`, seção
**Registry de transição — Rebaseline 2026**, preserva do ADR 004 somente o princípio de
**sessão MTProto durável**. A disposição `RETAIN-PRINCIPLE / REWRITE` mantém a necessidade
de durabilidade e exige que o mecanismo de armazenamento seja redecidido; o mecanismo
Tursogo do ADR histórico não é baseline para a rebaseline.

Na versão efetivamente usada, `github.com/gotd/td v0.161.0`, `session.Storage` expõe
somente `LoadSession` e `StoreSession` sobre bytes opacos. O próprio upstream registra
que a segurança dessa implementação é crítica: posse do material de sessão pode permitir
agir como o usuário/bot autenticado e, em algumas situações, decriptar mensagens
anteriores.

O `TursoSessionStorage` atual preserva esse contrato estreito: carrega/grava bytes opacos,
mapeia ausência para `session.ErrNotFound` e não interpreta o payload. A implementação
observável está em `internal/telegram/session.go` e o comportamento de ausência/round-trip
é coberto por `internal/telegram/session_test.go`.

### Peer cache

`PeerStore`, implementado em `internal/telegram/peers.go`, é um cache em memória de
`model.Peer` usado principalmente para obter `access_hash` ao construir peers de entrada
para histórico e mídia. O collector carrega o cache persistido antes do backfill. Se um
peer de canal não estiver presente, `FetchHistory` falha para aquele canal; o backfill
agrega a falha, mas `Collector.Run` trata falha de backfill como não fatal e continua para
a captura live.

A persistência observada também não corresponde a um store geral de peers do gotd:

- `internal/cli/channels.go` executa `repo.AddChannel` seguido de `repo.SavePeer` em
  operações independentes durante `channels add`;
- o mesmo `internal/cli/channels.go` chama somente `repo.RemoveChannel` em `channels remove`,
  enquanto `internal/storage/repository.go` implementa essa remoção apenas sobre `channels`;
- `internal/telegram/peers.go` implementa `PeerStore.FlushToDB`, e a busca do repositório
  atual encontra cobertura em testes, mas nenhum caller de produção;
- `PeerStore.LoadFromDB`, também em `internal/telegram/peers.go`, é exposto por
  `internal/telegram/client.go` e consumido pelo fluxo de mídia em
  `internal/telegram/media.go` para reconstruir o cache em memória.

Assim, uma linha em `peers` não significa "subscription ativa" nem representa uma
autoridade completa de peers Telegram. É um artefato operacional reutilizável que pode
sobreviver à remoção de uma assinatura.

## Boundary de segurança e concorrência

O fluxo `run`, em `internal/cli/run.go`, constrói o collector com um cliente Telegram e
também injeta `p.NewMediaClient(collectorRepo)`. Em `cmd/limiar/provider.go`,
`NewMediaClient` chama `p.newTelegramClient(repo)`, criando outro `telegram.Client` para o
`MediaClient`; `internal/telegram/media.go` usa esse client através de `runOnce` em
operações de mídia. Portanto, a implementação atual pode manter clients independentes que
compartilham a mesma persistência de sessão.

Isso torna coordenação de escrita uma questão material para qualquer estratégia futura de
sessão. Em `gotd/td v0.161.0`, cada `session.FileStorage` possui seu próprio mutex e
`StoreSession` usa `os.WriteFile(path, data, 0600)` diretamente; o próprio upstream mantém
um TODO sobre escrita robusta/rename. Instâncias distintas apontando para o mesmo path não
compartilham esse mutex.

Também não é neutro colocar a sessão no novo arquivo SQLite de Evidence. A partir desse
momento, snapshots/backup daquele banco passariam a carregar credencial MTProto. Além
disso, EXP-LIMIAR-017 validou execução de `internal/storage/sqlite` em worker Travis
Windows AMD64, Go 1.26.2 e `CGO_ENABLED=0`; o experimento não testou nem estabeleceu
equivalência entre bits POSIX `0600` e ACLs do Windows.

Essas observações não demonstram que `FileStorage` é inadequado nem que SQLite é
inadequado. Demonstram que a escolha envolve segurança, atomicidade/coordenação,
backup/recovery e suporte de plataforma e, portanto, não pode ser inferida apenas da
co-localização legada.

## Impacto arquitetural

A Evidence separa dois problemas que a topologia legada colocava no mesmo storage:

1. **Sessão:** credencial durável cujo contrato precisa considerar ausência explícita,
   overwrite seguro, coordenação entre clients, recovery/backup consciente de segredo e
   proteção adequada nas plataformas suportadas.
2. **Peers:** cache operacional cujo contrato ainda precisa esclarecer quais entradas
   sobrevivem a restart, de onde podem ser reconstruídas, sua relação com subscriptions e
   quando estado stale pode ser descartado ou renovado.

A Evidence disponível não sustenta migrar a tabela `sessions` legada para o novo SQLite
apenas por inércia, nem promover a tabela `peers` a source of truth ou atribuir a ela o
ciclo de vida de uma subscription apenas porque já existe fisicamente.

## Questões ainda abertas

Este Finding não escolhe storage de sessão e não define o contrato final do peer cache.
Permanecem em aberto as garantias concretas de escrita/recovery/coordenação da sessão e o
contrato de reconstrução/ownership dos peers. Essas questões exigem Evidence experimental
separada antes de qualquer Proposal ou Decision.
