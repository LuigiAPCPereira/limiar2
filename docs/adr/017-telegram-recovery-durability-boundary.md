# ADR 017 — Boundary durável de recovery Telegram

Authority: Decision Record
Status: Proposed

## Contexto

O ADR 016 aceitou os princípios de Evidence e sincronização Telegram, mas deixou a mecânica concreta de recovery fora de escopo.

O EXP-LIMIAR-001 e o F-ING-008 validaram `github.com/gotd/td v0.161.0` por inspeção e por 12 contract tests executados com Go 1.26.6 e race detector. Os testes confirmaram que erro de handler/storage não é, sozinho, uma fronteira confiável de parada; callbacks de `DifferenceTooLong` chegam depois de mudanças de state; bootstrap adota state remoto; e replay após restart é esperado.

Referências: ADR 016, PR #151, EXP-LIMIAR-001, F-ING-008 e PROPOSAL-ING-002.

## Decision proposta

### 1. gotd continua responsável pelo ordering/recovery

O Limiar não reimplementa `pts/qts/seq`. `updates.Manager` continua responsável pelo recovery especializado do Telegram.

### 2. O recovery é envolto por quatro responsabilidades

```text
Telegram RPC -> GuardedRecoveryAPI -> updates.Manager -> DurableEvidenceHandler -> Evidence Store
                                      |
                                      -> GuardedStateStorage

DurabilityBarrier + Supervisor atravessam API / Handler / StateStorage.
```

Os nomes são ilustrativos; os contratos são obrigatórios.

### 3. Evidence precede state correspondente

Se uma observação exige Evidence, sua persistência deve completar antes de o state correspondente poder ser certificado como avançado. Falha de Evidence fecha a barrier antes do retorno ao gotd, impede novos state writes e encerra o lifecycle do manager.

### 4. StateStorage é guardado de forma linearizável

Checagem da barrier e state write acontecem na mesma região crítica, sem TOCTOU. Falha de state write fecha a barrier e encerra o lifecycle, evitando continuar com state interno potencialmente divergente do persistido.

### 5. Recovery API intercepta bootstrap e descontinuidades

Antes de entregar ao manager uma resposta que estabelece novo baseline ou representa perda de continuidade, o Limiar registra Evidence correspondente. Casos mínimos:

- `UpdatesGetState` quando estabelece bootstrap sem state local;
- `UpdatesDifferenceTooLong`;
- `UpdatesChannelDifferenceTooLong`.

Se essa Evidence falhar, a resposta não chega ao manager.

### 6. TooLong é descontinuidade explícita

O Limiar pode adotar o novo baseline depois de registrar a descontinuidade, mas nunca afirma que observou os eventos que já não puderam ser recuperados.

### 7. Updates stateless não são autoridade de sync

PTS/QTS negativos ou sintéticos em envelopes reconstruídos não podem avançar `SourceSyncState`. A autoridade operacional permanece no recovery manager + StateStorage.

### 8. Replay é esperado

`Evidence durável + state persistido antigo` é condição segura. Restart pode reapresentar observações; downstream deve ser idempotente/reconciliável. Não há promessa de exactly-once.

### 9. A implementação pode variar sem mudar os contratos

Tipos, pacotes e nomes podem evoluir sem novo ADR se preservarem os contratos acima. Remover um boundary, alterar a semântica de interrupção, permitir state avançado sem Evidence exigida ou mudar a autoridade de sync exige nova Decision.

## Consequências

Benefícios: preserva os invariantes de durabilidade mesmo nos caminhos onde o gotd absorve erros; torna bootstrap/perdas auditáveis; mantém recovery especializado; e explicita restart/replay.

Custos: ingress ganha adapters e lifecycle explícitos; falhas de persistência preferem indisponibilidade a continuidade ambígua; downstream precisa tolerar replay.

## Fora do escopo

Engine SQLite/PRAGMAs, schema físico de Evidence, payload, session/peer storage, backoff entre restarts, Source Message Projection, implementação de backfill e processamento comercial.

## Gates antes de produção

1. preservar os 12 contratos já executados;
2. passar `go test -race`;
3. adicionar teste end-to-end de `ChannelDifferenceTooLong` com diálogo/PTS válido;
4. cobrir edit/delete/update composto no boundary real;
5. provar coexistência backfill/live sem compartilhar autoridade de progresso;
6. usar storage capaz de cumprir a ordem Evidence/state.

## Relação com ADR 016

Este ADR complementa, não supersede, o ADR 016. ADR 016 define as propriedades de Evidence/sync; este ADR define o boundary de recovery do gotd que as preserva.

## Escopo da eventual aceitação

A aceitação autoriza implementar essas responsabilidades em produção. Não aceita automaticamente engine de storage, schema físico de Evidence ou demais itens fora de escopo.