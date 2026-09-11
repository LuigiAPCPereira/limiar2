# EXP-LIMIAR-012 — WAL reader snapshot e locking do novo SQLite

Authority: Experiment
Status: Supported

## Pergunta

Na combinação concreta atualmente exercitada pelo Limiar com `github.com/ncruces/go-sqlite3`, um reader independente consegue manter um snapshot aberto em WAL sem bloquear um append de Evidence pelo writer de produção, preservando ao mesmo tempo isolamento de snapshot?

## Por que este experimento existe

O ADR 019 aceita `WAL + synchronous=FULL` e uma conexão lógica para o writer do novo storage, mas mantém locking/WAL como gate explícito por plataforma porque o driver usa uma VFS Go própria. O teste já existente prova que `journal_mode=WAL` é aplicado; ele não exercita a coexistência entre um reader independente e o writer enquanto uma transação de leitura mantém snapshot ativo.

O EXP-LIMIAR-011 reduziu a incerteza de backup/restore. Este experimento avança o outro gate independente ainda aberto do ADR 019 sem alterar `SourceSyncState`, `BackfillProgress`, schema, pool de produção ou política de concorrência.

## Hipótese

Com o banco aberto pelo `Store` real e um segundo handle read-only no mesmo arquivo:

1. ambos observam `journal_mode=wal`;
2. um read transaction que já materializou snapshot continua vendo o estado anterior;
3. o `EvidenceAppender` consegue persistir nova Evidence enquanto esse reader permanece ativo;
4. depois de encerrar o snapshot, o reader observa a Evidence recém-commitada.

Se o append bloquear até o timeout, falhar por locking inesperado, ou o snapshot mudar de conteúdo durante a mesma transação, a hipótese é rejeitada para o ambiente exercitado.

## Harness

`internal/storage/sqlite/wal_locking_experiment_test.go`:

- abre o banco pelo mesmo `Open` usado pelo novo storage;
- persiste uma Evidence inicial;
- abre um segundo handle `database/sql` em modo read-only para o mesmo arquivo;
- inicia uma transação read-only e força a criação do snapshot com `SELECT COUNT(*)`;
- tenta um novo append pelo `EvidenceAppender` com deadline explícita de 2 segundos;
- confirma que a mesma transação read-only continua vendo o snapshot antigo;
- após `COMMIT`, confirma que o observer vê a segunda Evidence.

O segundo handle existe somente no harness. Ele não muda a baseline de uma conexão lógica do writer e não autoriza aumentar o pool de produção.

## Evidência executada

No HEAD `6f9871942af65065f2a1a649e3053b62c33530a8`, o gate Travis CI passou em Linux Noble com Go 1.26.2. O pipeline executou, entre outros gates, `go vet ./...`, build completo e `go test -v -race -coverprofile=coverage.out -count=1 ./...` sem falha funcional ou race.

Nesse ambiente, o teste demonstrou as quatro propriedades da hipótese: `journal_mode=wal` no observer; snapshot de leitura estável durante o append; append de Evidence concluído enquanto o reader mantinha o snapshot; e visibilidade da nova Evidence depois do `COMMIT` do reader.

A evidência é deliberadamente limitada a Linux X64 no ambiente exercitado pelo Travis. Ela não autoriza generalizar locking/WAL para Windows, macOS, ARM64, Android/Termux ou PRoot.

## Critérios para `Supported`

O experimento só pode ser promovido de `In Progress` para `Supported` quando o HEAD correspondente executar em gate real e demonstrar todas as propriedades acima sem race/falha funcional.

Esse critério foi atendido pelo HEAD e ambiente registrados em **Evidência executada**.

## Fora de escopo

- múltiplos writers de produção;
- tuning de `busy_timeout`;
- throughput/benchmark;
- power-loss durante `fsync`;
- compartilhamento do banco por processos independentes em produção;
- política de backup/restore;
- mudança de `SetMaxOpenConns(1)`;
- autorização de `SourceSyncState` ou `BackfillProgress`.

## Próximo gate

Usar esta evidência para reduzir o gate 8 do ADR 019 somente para Linux X64. Repetir o boundary em plataformas adicionais antes de qualquer declaração de suporte nelas; nenhuma mudança de pool, política de concorrência ou schema é autorizada por este experimento.
