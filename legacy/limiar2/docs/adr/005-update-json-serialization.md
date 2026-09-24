# ADR 005 — Serializar Atualizações Brutas para JSON

## Status

Aceito

## Contexto

Todo o propósito da Fase 1 é descobrir o formato **real** dos dados
que o Telegram entrega, antes de desenhar qualquer normalização ou schema. Portanto,
precisamos capturar as atualizações sem perdas e em um formato fácil de inspecionar e
consultar mais tarde, sem nos comprometermos com um schema de domínio que ainda não
compreendemos.

Os tipos `tg` do gotd/td (ex: `tg.UpdatesClass`, `tg.Message`) são structs
Go simples com campos exportados, então o pacote `encoding/json` captura
toda sua estrutura.

## Decisão

Serializar cada atualização bruta para JSON e armazenar os bytes literalmente (verbatim). `encodeUpdate`
(`telegram/encode.go`) empacota (marshals) o `tg.UpdatesClass` originado no handler de atualizações
do gotd; `extractMessages` faz o mesmo para cada `*tg.Message` de respostas de histórico.
Os bytes JSON viram `RawMessage.Payload` e são gravados em `raw_messages.payload`
(TEXT) inalterados. Todo registro recebe a marcação `schema_version = 1` para que
fases futuras possam evoluir o formato de captura e conseguir distinguir as gerações de payload.

O `MessageHandler` do collector preserva os bytes originais (`Payload: update`) e
não os modifica; o `NoopClassifier` passa a mensagem inalterada adiante.

## Consequências

- Captura sem perdas, legível por humanos; os payloads podem ser consultados como JSON válido
  e inspecionados para desenhar o `limiar-processor`.
- `schema_version` provê um gancho (hook) de compatibilidade futura para alterar o
  formato da captura mais tarde.
- O JSON é maior do que uma codificação binária (ex: TL binário do gotd), mas o armazenamento é
  barato e a inspecionabilidade tem mais importância na Fase 1.
- Uma única atualização que falhe na codificação é logada e ignorada, ao invés de
  causar um crash no loop de recepção.

## Alternativas consideradas

- **Codificação binária TL do gotd** — compacta mas opaca; anula a finalidade de
  inspecionar os dados reais. Rejeitada.
- **Um schema de domínio normalizado agora** — prematuro; a Fase 1 existe exatamente para
  aprender o formato primeiro. Rejeitada.
- **Protobuf/MsgPack** — dependências adicionais e ferramentas extras que não oferecem benefício na Fase 1.
