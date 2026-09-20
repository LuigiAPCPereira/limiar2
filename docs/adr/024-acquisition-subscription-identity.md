# ADR 024 — Identidade de Acquisition Subscription

Authority: Decision Record
Status: Proposed

> Este ADR está `Proposed`. Ele não autoriza mudança de produção até aceitação explícita do mantenedor.

## Contexto

Os ADRs 016 e 018 tornam a admissão de Source Evidence anterior ao `updates.Manager` parte do boundary de ingress. O ADR 020, já `Accepted`, fixa `subscription_id` como campo obrigatório de cada Evidence e o define como o escopo configurado de aquisição que admitiu a observação.

Ao confrontar esses contratos com o runtime atual, surgiu uma lacuna concreta: o callback live recebe o envelope Telegram, mas o `telegram.Client` não recebe uma identidade de subscription configurada. O filtro legado de canais monitorados ocorre posteriormente no collector. Logo, o runtime atual não possui uma fonte autoritativa para preencher `Evidence.subscription_id` no boundary de Source Admission.

Não é correto preencher esse campo com `channel_id` apenas porque ambos identificam algum escopo. Canal é identidade da fonte; subscription é identidade da configuração de aquisição. Confundi-los faria a topologia/configuração legada criar uma nova identidade durável sem Decision.

Os ADRs Proposed 021, 022 e 023 não resolvem esta questão: tratam respectivamente de schema físico de `SourceSyncState`, `BackfillProgress` e sessão MTProto.

Experimentos anteriores usam valores como `telegram:test` apenas como fixture. Isso demonstra a necessidade do campo nos contracts, não define sua semântica de produção.

## Decision proposta

### 1. Subscription identifica uma configuração de aquisição, não uma mensagem nem um canal

`subscription_id` identifica de forma estável o escopo configurado que determina quais observações uma instância de aquisição admite.

Ele não é:

- `channel_id`;
- `message_id`;
- peer ID;
- session ID MTProto;
- posição de sync/backfill;
- identidade física de Evidence.

Uma subscription pode abranger uma ou várias fontes/canais conforme sua configuração.

### 2. A identidade nasce na configuração, antes do runtime Telegram

A camada que materializa a configuração de aquisição deve fornecer um `subscription_id` não vazio ao construir o boundary live.

O `telegram.Client`/Source Admission recebe essa identidade por injeção explícita. O callback de update não a deriva do envelope recebido.

A forma concreta de configuração (arquivo, tabela, CLI ou outra superfície) permanece detalhe de implementação enquanto preservar identidade estável e validação fail-closed.

### 3. Identidade estável é separada do conteúdo mutável da subscription

Alterar atributos operacionais de uma subscription não deve, por padrão, trocar sua identidade. Criar um escopo de aquisição semanticamente novo deve produzir nova identidade em vez de reutilizar silenciosamente uma anterior.

Este ADR não escolhe UUID, ULID ou outro formato físico para `subscription_id`; o contrato inicial é uma string opaca, estável e não vazia, compatível com o ADR 020.

### 4. Canais monitorados legados são input de migração/configuração, não identidade

A lista histórica de canais monitorados pode alimentar a configuração de uma subscription durante transição, mas não define por si só `subscription_id` nem ganha autoridade arquitetural sobre Source Admission.

Mover o filtro legado para antes da persistência exige preservar a semântica aceita de Evidence: nenhuma observação declarada admitida pode desaparecer silenciosamente. A política exata de quais envelopes são relevantes/admitidos deve ser testada no adapter real.

### 5. A mesma identidade correlaciona Evidence e authorities operacionais da mesma aquisição

Quando schemas de `SourceSyncState` e `BackfillProgress` forem aceitos/implementados, o mesmo `subscription_id` pode correlacionar essas authorities com a aquisição correspondente, sem torná-las a mesma authority.

Live sync, backfill e Evidence continuam com lifecycle e semântica próprios conforme ADRs 016/019.

### 6. Falta ou ambiguidade de subscription falha fechada

O boundary de produção não deve inventar `subscription_id` a partir de metadata do update. Configuração ausente, vazia ou ambígua deve impedir a inicialização/admissão correspondente com erro explícito.

## Consequências

### Positivas

- destrava o wiring correto de `Source Admission -> EvidenceAppender -> updates.Manager`;
- impede que `channel_id` vire identidade de configuração por conveniência;
- permite correlacionar Evidence, live sync e backfill sem misturar authorities;
- torna ownership da identidade anterior ao callback Telegram e testável;
- permite evolução da lista de canais sem reidentificar Evidence histórica por acidente.

### Negativas

- o runtime precisará transportar explicitamente contexto de subscription até o ingress;
- a transição da configuração legada exige mapping explícito;
- múltiplas subscriptions futuras exigirão lifecycle/configuração próprios;
- o formato físico definitivo da identidade permanece aberto.

## Alternativas rejeitadas

1. **Usar `channel_id` como `subscription_id`** — mistura identidade da fonte com identidade da configuração e não suporta naturalmente subscription multi-canal.
2. **Gerar uma identidade nova a cada processo/startup** — quebra correlação durável e continuidade operacional.
3. **Usar session ID/conta Telegram** — sessão é boundary operacional/segredo e pode servir múltiplos escopos de aquisição.
4. **Deixar o callback inferir a identidade** — o envelope não possui autoridade para decidir qual configuração o admitiu.
5. **Adiar o campo e gravar placeholder global** — cria identidade persistente artificial e torna migração posterior ambígua.

## Gates antes de implementação produtiva do wiring

1. configuração fornece `subscription_id` estável e não vazio antes de iniciar live ingress;
2. Source Admission recebe a identidade por injeção, nunca por inferência do update;
3. teste demonstra `append success -> forward` preservando o `subscription_id` configurado;
4. teste demonstra `append failure -> no forward`;
5. configuração ausente/ambígua falha antes de admitir Evidence;
6. nenhum caminho converte automaticamente `channel_id` em `subscription_id`;
7. comportamento de canais monitorados legados é coberto por teste durante a transição;
8. os gates de crash/restart e recovery dos ADRs 017–019 permanecem obrigatórios.

## Fora do escopo

Este ADR não decide:

- formato físico definitivo de `subscription_id`;
- UI/CLI final de gerenciamento de subscriptions;
- schema SQL de configuração;
- cardinalidade máxima de subscriptions por processo/conta;
- schemas de `SourceSyncState` ou `BackfillProgress`;
- session storage;
- política completa de filtros de eventos;
- migração do banco legado.

## Relação com Decisions existentes

- ADR 016 continua definindo Evidence/sync e separação de authorities;
- ADR 017 continua definindo recovery/fail-stop;
- ADR 018 continua exigindo Source Admission antes do `updates.Manager`;
- ADR 019 continua definindo o storage físico e ordering;
- ADR 020 continua definindo o envelope de Evidence, incluindo `subscription_id` obrigatório;
- ADRs 021–023 não são promovidos por esta Proposal.

## Escopo de eventual aceitação

A aceitação deste ADR autorizará somente a semântica e ownership de `subscription_id` descritas acima e o transporte explícito dessa identidade até Source Admission.

Ela não autorizará por si só schema novo de configuração, migração do legado, aceitação dos ADRs 021–023 nem remoção ampla do Dispatcher/collector legado.