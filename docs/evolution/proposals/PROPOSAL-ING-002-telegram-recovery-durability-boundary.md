# PROPOSAL-ING-002 — Boundary durável de recovery Telegram

Authority: Non-authoritative
Status: Ready

## Problema

O ADR 016 aceitou os princípios de Evidence e sincronização Telegram, mas deixou a mecânica concreta de recovery fora de escopo.

O EXP-LIMIAR-001 e o F-ING-008 demonstraram que integrar diretamente `updates.Manager -> UpdateHandler -> StateStorage` não é suficiente para preservar o contrato de durabilidade do Limiar:

- erros do handler e do `StateStorage` podem ser absorvidos pelo gotd;
- `DifferenceTooLong` e `ChannelDifferenceTooLong` avançam state antes dos callbacks;
- bootstrap sem state local adota um baseline remoto;
- `AuthOptions.Forget=true` também força substituição por state remoto;
- recovery pode entregar updates stateless, cujos PTS/QTS não são autoridade de sync.

A suíte experimental contra `github.com/gotd/td v0.161.0` em Go 1.26.6 passou 12 contract tests e `go test -race`.

A inspeção de `Manager.loadState` também confirmou que erro de `GetState` é propagado: falha de leitura não deve ser confundida com ausência de state.

## Evidência

- ADR 016;
- `EXP-LIMIAR-001-durable-telegram-update-recovery.md`;
- `F-ING-008-gotd-recovery-fail-stop-boundary.md`;
- PR #151;
- workflow experimental `agent-runtime` run `32171500698`;
- gotd v0.161.0 `telegram/updates/manager.go`.

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

Os nomes descrevem responsabilidades. Tipos/pacotes podem mudar sem nova Decision se os contratos forem preservados.

### 1. Evidence autoriza transições de state

Toda Evidence exigida pelo contrato de admissão de uma transição deve estar durável antes de o state correspondente avançar.

Isso não cria relação 1:1 entre Evidence e mensagem: updates compostos podem permanecer uma observação de fonte única e alimentar múltiplas projeções.

### 2. StateStorage guardado e linearizável

Barrier check + state write devem ocorrer na mesma região crítica. Falha de state write encerra o lifecycle para evitar continuar com state interno potencialmente divergente.

Erro de leitura de state é falha, não ausência. Nunca deve provocar bootstrap remoto por fallback.

### 3. Recovery API intercepta adoção de baseline

Antes de o manager receber uma resposta que estabeleça/substitua baseline ou represente descontinuidade, o Limiar registra Evidence correspondente:

- bootstrap sem state local;
- resync/reset explícito que substitua state local, inclusive fluxo equivalente a `Forget=true`;
- `UpdatesDifferenceTooLong`;
- `UpdatesChannelDifferenceTooLong`.

`Forget=true` não deve ser default operacional; somente ação explícita de resync/reset com Evidence.

### 4. Supervisor é a autoridade de interrupção

O sistema não depende da propagação de erro do gotd. Barrier fechada cancela o manager e impede novos state writes.

A barrier é terminal para aquela instância. Restart é uma nova tentativa partindo do state persistido antigo; a mesma barrier nunca é reaberta.

Política detalhada de backoff pode ser separada, mas loops apertados de restart não são aceitáveis.

### 5. Replay é esperado

`Evidence durável + state antigo` é seguro e pode gerar replay. Downstream deve ser idempotente/reconciliável. Não existe exactly-once.

### 6. Updates stateless não definem sync authority

PTS/QTS negativos/sintéticos de updates reconstruídos não avançam `SourceSyncState`. A autoridade de sync continua no recovery manager + StateStorage.

### 7. TooLong vira descontinuidade explícita

Quando o passado não é mais recuperável, o Limiar registra a descontinuidade e só então permite adoção do novo baseline. Isso não afirma observação dos eventos ausentes.

Replays podem repetir a Evidence da tentativa/descontinuidade; a reconciliação é derivada, não mutação do registro original.

## Alternativas rejeitadas

- apenas retornar erro no handler;
- apenas callbacks `OnTooLong` / `OnChannelTooLong`;
- reimplementar `pts/qts/seq`;
- continuar o mesmo lifecycle após falha de StateStorage;
- tratar erro de `GetState` como state ausente;
- usar `Forget=true` como reset silencioso/default.

## Trade-offs

Benefícios: preserva C-01/C-03, torna bootstrap/resync/perdas auditáveis, mantém recovery especializado no gotd e explicita replay.

Custos: adiciona adapters/lifecycle, prefere indisponibilidade a continuidade ambígua e exige downstream tolerante a replay.

## Fora do escopo

Engine SQLite/PRAGMAs, schema físico de Evidence, payload, session/peer storage, política detalhada de backoff, Source Message Projection e processamento comercial.

## Gates de implementação

Antes de produção:

1. storage escolhido deve cumprir Evidence + state ordering;
2. preservar os 12 contratos já validados;
3. `go test -race` obrigatório;
4. testar end-to-end `ChannelDifferenceTooLong` com diálogo/PTS válido;
5. testar resync explícito/`Forget=true` com Evidence antes da substituição do baseline;
6. provar que erro de leitura de state não cai em bootstrap;
7. testar edit/delete/update composto e coexistência backfill/live.

## Recomendação

Criar ADR complementar ao ADR 016 para tornar essas responsabilidades de recovery uma Decision explícita antes de implementar o novo ingress.