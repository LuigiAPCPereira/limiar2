# F-STO-006 — Sessão MTProto e peer cache possuem autoridades distintas

Authority: Non-authoritative Finding
Status: Open

## Descoberta

A implementação legada co-localiza `sessions` e `peers` no mesmo repositório Tursogo,
mas o comportamento observado mostra que esses estados possuem finalidade, criticidade e
lifecycle diferentes. Portanto, a coexistência física atual não sustenta tratá-los como
uma única autoridade futura nem copiar mecanicamente as duas tabelas para o novo storage.

### Sessão MTProto

O registry de transição preserva do ADR 004 somente o princípio de **sessão MTProto
durável** (`RETAIN-PRINCIPLE / REWRITE`). O mecanismo Tursogo do ADR histórico não é
baseline para a rebaseline.

Na versão efetivamente usada, `github.com/gotd/td v0.161.0`, `session.Storage` expõe
somente `LoadSession` e `StoreSession` sobre bytes opacos. O próprio upstream registra
que a segurança dessa implementação é crítica: posse do material de sessão pode permitir
agir como o usuário/bot autenticado e, em algumas situações, decriptar mensagens
anteriores.

O `TursoSessionStorage` atual preserva esse contrato estreito: carrega/grava bytes opacos,
mapeia ausência para `session.ErrNotFound` e não interpreta o payload.

### Peer cache

`PeerStore`, por outro lado, é um cache em memória de `model.Peer` usado principalmente
para obter `access_hash` ao construir peers de entrada para histórico e mídia. O collector
carrega o cache persistido antes do backfill. Se um peer de canal não estiver presente,
`FetchHistory` falha para aquele canal; o backfill agrega a falha, mas `Collector.Run`
trata falha de backfill como não fatal e continua para a captura live.

A persistência observada também não corresponde a um store geral de peers do gotd:

- `channels add` resolve o canal e executa `AddChannel` seguido de `SavePeer` como duas
  escritas independentes;
- `channels remove` remove a assinatura monitorada, mas não remove a linha de peer;
- `PeerStore.FlushToDB` existe e possui testes, porém não possui caller de produção no
  repositório atual;
- `LoadFromDB` é consumido pelo collector e por operações de mídia para reconstruir o
  cache em memória.

Assim, uma linha em `peers` não significa "subscription ativa" nem representa uma
autoridade completa de peers Telegram. É um artefato operacional reutilizável que pode
sobreviver à remoção de uma assinatura.

## Boundary de segurança e concorrência

O binário `run` cria um `telegram.Client` para o collector e outro `telegram.Client`
encapsulado no `MediaClient`. Downloads proativos podem usar `runOnce` no client de mídia
enquanto o client principal do collector está ativo. Ambos são construídos com acesso à
mesma sessão persistida.

Isso torna coordenação de escrita parte da futura estratégia de sessão. Em particular,
adotar diretamente `gotd/session.FileStorage` por instância não é uma substituição
mecânica do repositório atual: em `v0.161.0`, cada `FileStorage` possui seu próprio mutex
e `StoreSession` usa `os.WriteFile(path, data, 0600)` diretamente; o próprio upstream
mantém um TODO sobre escrita robusta/rename. Instâncias distintas apontando para o mesmo
path não compartilham esse mutex.

Também não é neutro colocar a sessão no novo arquivo SQLite de Evidence. A partir desse
momento, snapshots/backup daquele banco passariam a carregar credencial MTProto. Além
disso, EXP-LIMIAR-017 validou runtime SQLite em Windows, mas explicitamente não provou
que o `0600` Unix possui uma garantia equivalente de ACL no Windows.

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
`peers` também não deve ser promovida a source of truth nem receber lifecycle de
subscription apenas por existir.

## Próximo passo

Investigar a estratégia de **sessão** primeiro, por ser credencial necessária para a
integração Telegram e possuir o maior impacto de segurança. Comparar candidatos em um
experimento isolado antes de propor uma Decision, cobrindo no mínimo:

- ausência vs. erro;
- round-trip e overwrite de bytes opacos;
- persistência após reopen/restart;
- falha durante escrita sem aceitar estado silenciosamente corrompido;
- coordenação quando mais de um client usa a mesma sessão;
- permissões/ACL nas plataformas que se pretende suportar;
- consequência para backup/restore e exposição de segredo.

A estratégia de peer cache deve ser investigada separadamente depois que seu contrato de
reconstrução e ownership estiver explícito.
