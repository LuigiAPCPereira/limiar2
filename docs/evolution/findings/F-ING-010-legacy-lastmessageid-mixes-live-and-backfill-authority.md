# F-ING-010 — `LastMessageID` legado mistura autoridade live e backfill

Authority: Non-authoritative

## Observado

O collector legado usa o mesmo campo persistido `channels.last_message_id` em dois
lifecycles diferentes.

No backfill:

- `internal/collector/backfill.go` lê `Channel.LastMessageID` como cursor;
- a primeira página usa esse valor como `MinID` de `messages.getHistory`;
- ao final de `backfillChannel`, o orquestrador chama
  `Repository.UpdateChannelLastMessage`.

No fluxo live:

- `internal/collector/dbwriter.go` chama o mesmo
  `Repository.UpdateChannelLastMessage` para mensagens live efetivamente inseridas;
- o batch consolida o maior `MessageID` por canal e escreve no mesmo campo.

A persistência concreta em `internal/storage/repository.go` implementa ambos como:

```sql
UPDATE channels
SET last_message_id = ?, last_collected_at = ?
WHERE id = ?
```

Portanto o storage não expressa duas authorities: há apenas um cursor compartilhado.

## Finding adicional de durabilidade

O backfill não espera confirmação de durabilidade de cada `WriteJob` antes de considerar
a página varrida.

`backfillChannel` envia o job para `c.writeCh` e continua. Quando retorna, `backfill`
pode chamar `UpdateChannelLastMessage`, enquanto o `dbWriter` é uma goroutine separada
que acumula jobs e só depois executa o flush.

Assim, a topologia permite a ordem observável:

```text
job histórico aceito na fila
        ↓
backfillChannel retorna
        ↓
LastMessageID avança
        ↓
commit da raw message ainda pendente ou pode falhar
```

Isso não prova que toda execução perde mensagens. Prova que o cursor legado não possui
um boundary que torne impossível certificar progresso histórico antes da Evidence
correspondente estar durável.

## Impacto

O desenho conflita com o ADR 016 já Accepted:

- live sync deve usar estado nativo do Telegram, não `LastMessageID`;
- backfill deve possuir progresso próprio;
- posição de history não certifica ausência de gaps live;
- progresso não pode certificar Evidence ainda não durável.

Também torna impossível atribuir significado único a `last_message_id`: dependendo do
último escritor, ele pode refletir uma observação live ou uma varredura histórica.

## O que este finding não decide

Este documento não define:

- schema físico de `BackfillProgress`;
- engine de storage;
- política de retenção histórica;
- tamanho/paginação do backfill;
- se live e backfill executam simultaneamente ou em fases do mesmo processo;
- formato físico de Evidence.

A conclusão observável é somente que **uma authority persistida compartilhada não
representa corretamente os dois lifecycles** e que a ordem atual entre enqueue histórico
e avanço do cursor não constitui prova de durabilidade.

## Evidência primária no repositório

- `internal/collector/backfill.go`
- `internal/collector/dbwriter.go`
- `internal/collector/collector.go`
- `internal/storage/repository.go`
- ADR 016 — Source Evidence e sincronização Telegram
