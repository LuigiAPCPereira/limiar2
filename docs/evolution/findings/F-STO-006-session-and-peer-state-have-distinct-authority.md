# F-STO-006 — Sessão MTProto e peer cache possuem autoridades distintas

Authority: Non-authoritative Finding
Status: Open

## Descoberta

A implementação legada co-localiza `sessions` e `peers` no mesmo banco de dados Tursogo,
mas o comportamento observado mostra que esses estados possuem finalidade, criticidade e
ciclo de vida diferentes. Portanto, a coexistência física atual não sustenta tratá-los como
uma única autoridade futura nem copiar mecanicamente as duas tabelas para o novo storage.

### Sessão MTProto

O registry de transição preserva do ADR 004 somente o princípio de **sessão MTProto
durável**: `RETAIN-PRINCIPLE / REWRITE` significa preservar o princípio arquitetural,
mas reescrever o mecanismo legado durante a rebaseline, conforme `docs/adr/README.md`.
O mecanismo Tursogo do ADR histórico não é baseline para a rebaseline.

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

Isso torna coordenação de escrita parte da futura estratégia de sessão. Em particular,
adotar diretamente `gotd/session.FileStorage` por instância não é uma substituição
mecânica do storage atual: em `v0.161.0`, cada `FileStorage` possui seu próprio mutex e
`StoreSession` usa `os.WriteFile(path, data, 0600)` diretamente; o próprio upstream
mantém um TODO sobre escrita robusta/rename. Instâncias distintas apontando para o mesmo
path não compartilham esse mutex.

Também não é neutro colocar a sessão no novo arquivo SQLite de Evidence. A partir desse
momento, snapshots/backup daquele banco passariam a carregar credencial MTProto. Além
disso, EXP-LIMIAR-017 validou execução de `internal/storage/sqlite` em Windows AMD64 no
ambiente observado, mas não testou nem estabeleceu equivalência entre bits POSIX `0600` e
ACLs do Windows.

Essas observações não demonstram que `FileStorage` é inadequado nem que SQLite é
inadequado. Demonstram que a escolha envolve segurança, atomicidade/coordenação,
backup/recovery e suporte de plataforma e, portanto, precisa de decisão explícita.

## Impacto arquitetural

A próxima estratégia não deve usar "session/peer state" como um único problema apenas
porque o legado os guarda no mesmo banco.

Devem permanecer perguntas independentes:

1. **Sessão:** onde e como persistir o blob sensível do gotd com ausência explícita,
   overwrite seguro, coordenação entre clients, recovery/backup consciente de segredo e
   proteção adequada nas plataformas suportadas?
2. **Peers:** quais peers realmente precisam sobreviver a restart, de onde podem ser
   reconstruídos, qual relação possuem com subscriptions e quando um cache stale pode ser
   descartado ou renovado?

A tabela `sessions` legada não deve ser migrada para o novo SQLite por inércia. A tabela
`peers` também não deve ser promovida a source of truth nem receber ciclo de vida de
subscription apenas por existir.

## Próximo passo

Investigar a estratégia de **sessão** primeiro, por ser credencial necessária para a
integração Telegram e possuir o maior impacto de segurança. O experimento correspondente
deve possuir critérios mensuráveis para ausência vs. erro, round-trip/overwrite,
reopen/restart, falha de escrita, coordenação entre clients, permissões/ACL e implicações
de backup/restore.

EXP-LIMIAR-018 materializa o primeiro teste desse boundary contra o candidato mais simples,
`gotd/session.FileStorage` usado as-is. O Finding não antecipa seu resultado nem transforma
esses critérios em Decision.

A estratégia de peer cache deve ser investigada separadamente depois que seu contrato de
reconstrução e ownership estiver explícito.
