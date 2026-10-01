# Registro de aceitação — L3 ADR 006 / identidade de Acquisition Subscription

**Data local do mantenedor:** 2026-10-01  
**Escopo:** Limiar 3.0 — L3-004 / identidade de aquisição necessária para ligar Source Admission ao ingress produtivo.  
**Decision a formalizar:** `docs/limiar3/adr/006-acquisition-subscription-identity.md`.

## Autorização explícita do mantenedor

Na conversa do projeto, após revisar a função de `subscription_id` e sua distinção de identidade de mensagem, canal, autorização Telegram e target MCP, o mantenedor determinou: **“então vamos resolver ela”**.

Esta autorização resolve o bloqueio arquitetural da identidade de Acquisition Subscription, mas não autoriza merge, deploy, Telegram real ou outras Decisions pendentes.

## Decisão aceita

1. `subscription_id` identifica uma **configuração de aquisição** do Limiar.
2. É uma string opaca, estável, não vazia e não secreta, criada/fornecida pela configuração antes de iniciar o ingress.
3. Não é `channel_id`, `message_id`, peer ID, `TelegramAuthorizationIdentity`, MTProto session ID nem nome de target MCP.
4. Source Admission recebe essa identidade por injeção explícita; nenhum adapter a infere do update.
5. Ausência, vazio ou ambiguidade de identidade falha fechado antes de admitir Evidence.
6. Alterações operacionais na composição de fontes não trocam a identidade por padrão; uma aquisição semanticamente nova recebe identidade nova.
7. A primeira implementação pode compor somente a cardinalidade necessária ao MVP sem criar framework de gerenciamento de subscriptions. Isso não autoriza um ID implícito/global: a identidade continua explicitamente configurada.
8. O mecanismo concreto de configuração (arquivo, env, CLI, tabela ou outro) e o formato físico definitivo do ID permanecem detalhes separados enquanto preservarem este contrato.

## Efeito sobre o caminho do MVP

Esta Decision libera o próximo slice de L3-004 para transportar uma identidade configurada até o core já validado de Source Admission:

```text
Acquisition configuration
        ↓
subscription_id explícito
        ↓
Telegram ingress adapter
        ↓
Evidence contextualizada
        ↓
Source Admission
        ↓
Append Evidence
        ↓
forward
```

Ela não exige um sistema completo de gerenciamento de subscriptions, UI administrativa ou schema adicional de banco para o MVP.

## Segurança e limites

- `subscription_id` não é segredo e não deve conter credenciais, session bytes, telefone, OTP, senha ou access hash;
- a identidade de aquisição não amplia o read scope MCP;
- o target MCP continua sendo authority distinta;
- a autorização Telegram continua authority distinta;
- não há autorização para derivar a identidade de dados não confiáveis recebidos do Telegram.

## Fora do escopo desta aceitação

Esta Decision não autoriza nem decide:

- schema de `SourceSyncState`;
- schema de `BackfillProgress`;
- durability barrier/supervisor;
- policy completa de filtros de eventos;
- migração do legado;
- UI/CLI final para subscriptions;
- armazenamento persistente específico da configuração;
- formato UUID/ULID definitivo;
- Telegram real, OTP/2FA, Secure MCP Tunnel, merge ou deploy.

## Referência

Este registro fornece a referência durável exigida por `AGENTS.md` para a promoção da Decision L3 de identidade de Acquisition Subscription para `Accepted`.
