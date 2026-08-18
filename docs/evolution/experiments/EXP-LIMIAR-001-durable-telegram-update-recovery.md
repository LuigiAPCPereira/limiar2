# EXP-LIMIAR-001 — Recuperação durável de updates Telegram

Authority: Non-authoritative
Status: Supported with limitations

## Hipótese

É possível integrar o mecanismo de recuperação/ordenação de updates do gotd com uma
barreira de durabilidade do Limiar de forma que o estado operacional de sincronização
não seja persistido como avançado enquanto uma Evidence admitida correspondente ainda
não tiver atingido a durabilidade exigida.

A propriedade desejada é evitar o estado proibido:

```text
Evidence não durável / sync state avançado
```

sem exigir semântica de exactly-once.

---

## Contexto

A arqueologia do ingress atual encontrou riscos materiais:

- live updates podem ser descartados sob backpressure;
- backfill pode avançar checkpoint antes da durabilidade do raw;
- uma falha em mensagem anterior pode ser atravessada por um cursor posterior;
- edits colidem com a identidade física atual `(channel_id, message_id)`;
- deletes não são persistidos como Evidence;
- `LastMessageID` mistura progresso histórico com corretude de sincronização live.

Telegram não define corretude de updates por maior ID de mensagem. A documentação
oficial usa estado de sequência como `pts`, `qts`, `seq` e estado específico de canais,
com recuperação via `updates.getDifference` e `updates.getChannelDifference`.

O gotd atual possui pacote de recuperação de updates e seu exemplo oficial configura
storage persistente para `qts/pts` e conecta o recovery manager ao `UpdateHandler` do
cliente.

---

## Fontes primárias consultadas

- Telegram — Working with Updates:
  https://core.telegram.org/api/updates
- Telegram — `updates.getDifference`:
  https://core.telegram.org/method/updates.getDifference
- gotd/td — exemplo oficial de userbot com `telegram/updates` e StateStorage:
  https://github.com/gotd/td/blob/main/examples/userbot/main.go
- gotd/td — arquitetura do pacote `telegram/updates`:
  https://github.com/gotd/td/blob/main/ARCHITECTURE.md

Versão atualmente pinada no Limiar durante a materialização deste documento:
`github.com/gotd/td v0.161.0`.

---

## Modelo experimental

O protótipo separou quatro papéis:

```text
Telegram update recovery
        ↓
Telegram Adapter
        ↓
DurableEvidenceHandler
        ↓
Evidence Store

StateStorage
        ↑
DurabilityBarrier / GuardedStateStorage
```

A ideia não é ensinar o Limiar a reimplementar `pts/qts/seq`. O recovery manager da
biblioteca continua responsável por ordenação e gap recovery; o Limiar adiciona um
gate para impedir que a persistência de estado certifique durabilidade inexistente.

### Estados considerados

| Evidence | Sync state | Interpretação |
| --- | --- | --- |
| não | não | falha antes da admissão/durabilidade; aceitável |
| sim | não | replay possível; aceitável |
| sim | sim | caminho normal |
| não | sim | **proibido** |

---

## Protótipo

Foi criado protótipo descartável fora do repositório de produção.

A primeira versão continha um TOCTOU: checar a barreira e escrever o state eram duas
operações separadas. Uma falha de Evidence poderia ocorrer entre as duas.

A segunda versão corrigiu isso com uma operação conceitual:

```go
GuardStateWrite(func() error {
    return underlyingStateStorage.Write(...)
})
```

mantendo o lock de leitura da barreira durante toda a escrita de state.

O protótipo final passou testes concorrentes com race detector no ambiente experimental.

---

## Resultado

**Supported**, para a propriedade local testada.

O experimento demonstrou que uma barrier linearizável pode impedir a combinação
`Evidence não durável / state avançado` no adapter do Limiar, preservando o caso
`Evidence durável / state antigo` como replay aceitável.

A conclusão suportada é:

> uma barreira explícita entre durabilidade de Evidence e persistência do state é uma
> estratégia viável para o boundary do Limiar.

Ela não prova que a implementação de produção já está correta.

---

## O que o experimento NÃO prova

- não prova exactly-once;
- não prova ausência de chamadas externas duplicadas após crash;
- não prova o comportamento de todos os caminhos internos do recovery manager do gotd;
- não valida ainda, contra a versão real pinada e toolchain final, todos os casos de
  handler error, state error, common difference, channel difference, cancel e restart;
- não decide formato físico de Evidence;
- não decide engine de storage;
- não decide session storage;
- não autoriza mudança de produção.

A semântica pretendida é **at-least-once observation + replay + downstream idempotente**.

---

## Próxima validação necessária

Antes de aceitar um ADR que fixe o mecanismo concreto de integração com gotd, executar
contract tests contra a versão real escolhida cobrindo pelo menos:

1. handler falha antes de Evidence durável;
2. Evidence commitada e state write falha;
3. gap comum e `getDifference`;
4. gap de canal e `getChannelDifference`;
5. cancelamento durante persistência;
6. restart após estado ambíguo;
7. replay da mesma observação.

---

## Conclusão epistemológica

`Supported` significa que a hipótese da barrier sobreviveu ao protótipo.

Não significa `Accepted`, não transforma `updates.New`/manager em arquitetura obrigatória
e não autoriza alterar o ingress de produção sem Decision apropriada.