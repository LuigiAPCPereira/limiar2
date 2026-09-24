# Registro de aceitação — repository rebaseline / legacy containment

**Accepted-by:** Mantenedor do Limiar
**Accepted-at:** 2026-09-23
**Escopo:** Limiar 3.0 — topologia do repositório anterior à L3-002.

## Decisão aceita

O mantenedor autorizou executar a reorganização proposta para que o root do repositório represente exclusivamente a geração atual Limiar 3, preservando a implementação anterior sob `legacy/limiar2/`.

A autorização cobre:

- branch dedicada de rebaseline;
- mover a implementação sobrevivente L1/L2/Limiar 2 para `legacy/limiar2/` preservando blobs/história Git;
- manter fontes constitucionais e documentação L3 canônica no root;
- novo módulo Go limpo no root para Limiar 3;
- preservar o módulo legado autocontido sob `legacy/limiar2/`;
- impedir dependência de código L3 -> legacy;
- substituir CI legado por CI mínimo/reprodutível do root L3;
- registrar a decisão como L3 ADR 003;
- atualizar tracker/checkpoint/handoff.

## Limites

Esta autorização não implementa L3-002, não conecta Telegram, não solicita OTP/2FA, não cria credenciais e não faz merge/deploy. O legado deve ser preservado como Evidence e referência; código L1 independente que não esteja presente no repositório não deve ser inventado.