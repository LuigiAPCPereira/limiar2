# PROPOSAL-ING-002 — Boundary durável de recovery Telegram

Authority: Non-authoritative
Status: Ready

## Problema

O ADR 016 aceitou os princípios de Evidence e sincronização Telegram, mas deixou a
mecânica concreta de recovery fora de escopo.

O EXP-LIMIAR-001 e o F-ING-008 demonstraram que integrar diretamente
`updates.Manager -> UpdateHandler -> StateStorage` não é suficiente para preservar o
contrato de durabilidade do Limiar:

- erros do handler e do `StateStorage` podem ser absorvidos pelo gotd;
- `DifferenceTooLong` e `ChannelDifferenceTooLong` avançam state antes dos callbacks;
- bootstrap sem state local adota um baseline remoto;
- recovery pode entregar updates stateless, cujos PTS/QTS não são autoridade de sync.

A suíte experimental executada contra `github.com/gotd/td v0.161.0` em Go 1.26.6
passou 12 contract tests e `go test -race`.

## Evidência

- ADR 016 — princípios de Source Evidence e sincronização Telegram;
- `EXP-LIMIAR-001-durable-telegram-update-recovery.md`;
- `F-ING-008-gotd-recovery-fail-stop-boundary.md`;
- PR #151 — harness e EvolutionDocs;
- execução experimental `agent-runtime` workflow run `32171500698`.

## Proposta

Adotar um boundary composto em torno do recovery do gotd:

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
atravessam API / Handler / StateStorage
```

Os nomes acima descrevem responsabilidades. A implementação pode ajustar tipos,
pacotes e nomes sem novo ADR desde que preserve os contratos abaixo.

### 1. Durable Evidence antes de state correspondente

Observações normais admitidas pelo handler devem atingir a durabilidade exigida antes
de um state write correspondente poder ser certificado como avançado.

Falha de Evidence fecha a barrier antes de retornar ao gotd.

### 2. StateStorage guardado e linearizável

A verificação de barrier + state write precisa ocorrer dentro da mesma seção crítica,
sem janela TOCTOU.

Se um state write falhar, o lifecycle entra em fail-stop. Isso evita continuar usando
state interno potencialmente divergente do state persistido.

### 3. Recovery API interceptada antes do manager

Respostas que representam adoção ou perda de continuidade devem ser materializadas como
Evidence antes de serem entregues ao `updates.Manager`:

- bootstrap por `UpdatesGetState`;
- `UpdatesDifferenceTooLong`;
- `UpdatesChannelDifferenceTooLong`.

Se a Evidence correspondente falhar, a resposta não chega ao manager.

### 4. Supervisor é a autoridade de fail-stop

O sistema não depende de propagação de erro do gotd para interromper o recovery.

Barrier fechada cancela o lifecycle do manager e impede novos state writes. Restart
posterior parte do state persistido durável.

### 5. Replay é esperado

`Evidence durável + state antigo` é condição segura e pode gerar replay após restart.
O downstream deve ser idempotente/reconciliável.

Não existe promessa de exactly-once.

### 6. Updates stateless não definem sync authority

PTS/QTS presentes em updates reconstruídos pelo recovery não devem ser usados pelo
domínio para reconstruir ou avançar `SourceSyncState`.

A autoridade operacional de sync permanece no recovery manager + StateStorage.

### 7. TooLong vira descontinuidade explícita

Uma ocorrência `*DifferenceTooLong` significa que o passado ausente pode não ser mais
recuperável. O Limiar registra a descontinuidade e pode adotar o novo baseline depois
dessa Evidence, sem afirmar que observou os eventos perdidos.

## Alternativas rejeitadas

### Apenas retornar erro no handler

Rejeitada: o gotd pode registrar o erro e continuar.

### Apenas callbacks `OnTooLong` / `OnChannelTooLong`

Rejeitada: os callbacks ocorrem depois do avanço de state nos caminhos relevantes.

### Reimplementar `pts/qts/seq`

Rejeitada: duplica a máquina especializada do gotd sem necessidade.

### Permitir continuidade após falha de StateStorage

Rejeitada no baseline atual: o state interno pode divergir do persistido, tornando o
lifecycle ambíguo. Fail-stop é mais simples e seguro.

## Trade-offs

### Benefícios

- preserva C-01/C-03 mesmo quando o gotd absorve erros;
- torna bootstrap e perdas irrecuperáveis auditáveis;
- mantém recovery especializado no gotd;
- replay fica explícito e testável;
- evita usar payload stateless como falsa autoridade.

### Custos

- adiciona três boundaries/adapters e supervisor explícito;
- indisponibilidade é preferida a continuar após falha de durabilidade;
- downstream precisa tolerar replay;
- Evidence de sync precisa de contrato físico ainda não decidido.

## Fora do escopo

Esta Proposal não decide:

- engine SQLite ou PRAGMAs;
- schema físico de Evidence;
- formato do payload;
- session/peer storage;
- política de backoff/retry após restart;
- Source Message Projection;
- processamento comercial.

## Gates de implementação

Antes de produção:

1. storage escolhido deve conseguir implementar os contratos de Evidence + state;
2. testes de integração devem manter os 12 contratos já validados;
3. adicionar caso end-to-end de `ChannelDifferenceTooLong` com diálogo/PTS válido;
4. testar edit/delete/update composto e coexistência backfill/live no boundary real;
5. `go test -race` obrigatório para o pacote de ingress/recovery.

## Recomendação

Criar ADR complementar ao ADR 016 para tornar essas responsabilidades de recovery uma
Decision explícita antes de implementar o novo ingress.