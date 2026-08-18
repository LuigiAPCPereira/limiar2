# EXP-LIMIAR-001 — Recuperação durável de updates Telegram

Authority: Non-authoritative
Status: Supported with limitations; runtime contract confirmed

## Hipótese

É possível integrar o recovery/ordering do gotd com um boundary de durabilidade do
Limiar de forma que o sync state persistido não certifique uma observação, bootstrap ou
descontinuidade cuja Evidence exigida ainda não esteja durável.

Estado proibido:

```text
Evidence exigida não durável / sync state persistido como avançado
```

A hipótese não exige exactly-once.

## Contexto

A arqueologia do ingress encontrou perda sob backpressure, avanço prematuro de cursor,
colisão entre revisões, ausência de Evidence para deletes e uso de `LastMessageID` como
uma autoridade que o protocolo Telegram não define.

Telegram usa `pts/qts/seq/date` e estado de canais, com recovery por
`updates.getDifference`/`updates.getChannelDifference`.

Versão pinada examinada: `github.com/gotd/td v0.161.0`.

## Fontes primárias

- Telegram — Working with Updates: https://core.telegram.org/api/updates
- Telegram — `updates.getDifference`: https://core.telegram.org/method/updates.getDifference
- gotd/td v0.161.0 — `telegram/updates/manager.go`
- gotd/td v0.161.0 — `telegram/updates/state.go`
- gotd/td v0.161.0 — `telegram/updates/state_apply.go`
- gotd/td v0.161.0 — `telegram/updates/state_channel.go`
- gotd/td v0.161.0 — `telegram/updates/sequence_box.go`
- gotd/td v0.161.0 — `telegram/updates/storage.go`

## Fase A — protótipo abstrato da barrier

O protótipo descartável inicial modelou:

```text
DurableEvidenceHandler
        ↓
Evidence Store

GuardedStateStorage
        ↑
DurabilityBarrier
```

A primeira versão possuía TOCTOU entre checar a barrier e escrever state. A versão
corrigida mantém verificação + state write na mesma seção crítica.

Resultado: **SUPPORTED** para a propriedade local de impedir
`Evidence não durável / state avançado`.

## Fase B — inspeção do gotd v0.161.0

A leitura do source pinado invalidou a suposição de que erro de handler/storage encerraria
necessariamente o manager.

Também confirmou que:

- `UpdatesDifferenceTooLong` avança PTS antes de `OnTooLong`;
- `UpdatesChannelDifferenceTooLong` avança channel PTS antes de `OnChannelTooLong`;
- ausência de state local leva à adoção de state remoto via `UpdatesGetState`;
- updates reconstruídos por difference podem chegar stateless ao handler.

Finding detalhado:
`docs/evolution/findings/F-ING-008-gotd-recovery-fail-stop-boundary.md`.

O candidato simples foi rejeitado. O Candidate v3 passou a ser:

```text
Telegram RPC
    ↓
GuardedRecoveryAPI
    ↓
updates.Manager
    ↓
DurableEvidenceHandler
    ↓
Evidence Store

updates.Manager
    ↓
GuardedStateStorage

DurabilityBarrier + Supervisor
```

## Fase C — contract tests reais

A PR `limiar-collector#151` preparou 12 contratos cobrindo:

1. Evidence durável -> PTS pode avançar;
2. falha de Evidence -> PTS persistido antigo;
3. falha de state write -> barrier fecha;
4. barrier -> supervisor cancela Manager;
5. common gap -> segundo `getDifference` após startup;
6. falha durante `getDifference` -> state antigo;
7. callback `DifferenceTooLong` é tardio;
8. guard bloqueia common/channel TooLong se Evidence falha;
9. bootstrap remoto não é adotado se sua Evidence falha;
10. `UpdateChannelTooLong` -> novo `getChannelDifference`;
11. falha -> restart -> replay do state persistido antigo;
12. replay de difference chega stateless (`Pts=-1`).

Como o Actions do Limiar falhava antes de qualquer step, a mesma suíte foi executada em
módulo descartável no `agent-runtime#14`, exclusivamente para usar o runner self-hosted
já existente.

Ambiente:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
gotd/td: v0.161.0
Workflow run: 32171500698
```

Resultados:

```text
go test ./... -count=1 -v       -> PASS (12 contratos)
go test -race ./... -count=1    -> PASS
```

Portanto:

```text
barrier abstrata: SUPPORTED
source contract v0.161.0: CONFIRMED
runtime contract exercitado: CONFIRMED
Candidate v3: SUPPORTED
```

## O que o experimento NÃO prova

- exactly-once;
- recuperação de eventos que o Telegram já não oferece;
- ausência de chamadas externas duplicadas após crash;
- schema físico de Evidence;
- engine/PRAGMAs de storage;
- session storage;
- policy final de retries/supervisor;
- correção da integração completa antes de ela existir no Limiar;
- autorização para mudança de produção.

A garantia pretendida permanece **at-least-once observation dentro do boundary
controlado + replay + downstream idempotente**, com descontinuidades explícitas quando a
fonte não consegue mais recuperar o passado.

## Pendências antes de produção

1. definir schema das Evidence de bootstrap/descontinuidade junto ao storage;
2. definir lifecycle/retry do supervisor após barrier fechada;
3. preservar esses contract tests na implementação real do adapter;
4. validar a integração completa do ingress no módulo do Limiar.

## Conclusão epistemológica

O Candidate v3 sobreviveu ao protótipo, à inspeção do gotd pinado e à execução real com
race detector.

Isso eleva a mecânica para **Candidate suportado**, não para Decision. `Supported` não
significa `Accepted` e não autoriza produção sem a governança correspondente.
