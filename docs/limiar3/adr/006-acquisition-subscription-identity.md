# L3 ADR 006 — Identidade e lifecycle de Acquisition Subscription

Authority: Decision Record — Limiar 3.0  
Status: Accepted  
Accepted-by: Mantenedor do Limiar  
Accepted-at: 2026-10-01  
Acceptance-reference: `0ce7d88f2b47db397ae6463d9857c5ca9212e013`

## Contexto

O Limiar 3 separa Telegram/MTProto, MCP realtime, coleta durável, Evidence, live recovery,
backfill e processamento comercial. Os ADRs históricos 016–020, explicitamente herdados
para L3-004, exigem que Source Evidence seja persistida antes do avanço certificado da
fonte e que cada registro de Evidence contenha um `subscription_id` não vazio.

O kernel de Evidence de L3-004/S5 e o core genérico de Source Admission de S6 já estão
implementados e validados em CI. O S6 recebe uma `evidence.Evidence` já contextualizada,
faz append e somente então permite o forward. O ponto ainda aberto era quem possui e
fornece a identidade de aquisição que entra em `Evidence.subscription_id`.

O runtime Telegram possui `TelegramAuthorizationIdentity`, que identifica o owner de uma
autorização/credencial Telegram. O MCP realtime possui targets nomeados, que delimitam seu
read scope. A própria mensagem possui identidade na fonte. Nenhuma dessas identidades é a
identidade da configuração que decidiu admitir aquela observação no corpus durável.

O ADR histórico 024 descreveu esta lacuna como Proposal e rejeitou derivar
`subscription_id` de `channel_id`, session ID ou fixture. O mantenedor resolveu
explicitamente a questão para Limiar 3 e autorizou que a Decision feche também lifecycle,
ownership, correlação, evolução e failure semantics, em vez de criar apenas um atalho de
MVP.

## Decision

### 1. `subscription_id` identifica uma configuração de aquisição

`subscription_id` é a identidade estável da **Acquisition Subscription**: a configuração
que define um escopo de aquisição e sob a qual uma observação é admitida como Source
Evidence.

Ela não é:

- identidade de mensagem;
- `channel_id`, peer ID ou username;
- `TelegramAuthorizationIdentity`;
- MTProto session ID;
- nome de target MCP;
- cursor, `pts/qts/seq`, posição de backfill;
- identidade física da própria Evidence.

A mesma fonte pode participar de scopes diferentes sem que a identidade da fonte se
confunda com a identidade da aquisição.

### 2. A identidade nasce antes do ingress e é injetada explicitamente

A camada de configuração/composição materializa a Acquisition Subscription antes de
instalar o ingress que pode admitir Evidence.

O caminho autorizado é:

```text
Acquisition configuration
        ↓
subscription_id explícito
        ↓
classificação/roteamento do ingress
        ↓
Evidence contextualizada
        ↓
Source Admission
        ↓
Append Evidence
        ↓
forward
```

O callback Telegram, o Source Admission core, o storage e o MCP não inventam essa
identidade.

Ausência, valor vazio/whitespace-only ou ambiguidade de identidade falha fechado antes de
uma observação ser declarada admitida.

### 3. Forma lógica: string opaca, estável, não secreta

O contrato de L3 usa uma string opaca:

- não vazia;
- estável entre restarts/redeploys da mesma aquisição;
- comparada como identidade, não como label de apresentação;
- não secreta;
- sem credenciais, telefone, OTP, senha, auth key, access hash ou session bytes;
- não derivada de payload Telegram ou outro dado não confiável.

Esta Decision não fixa UUID, ULID, slug ou outro formato físico permanente. Um gerador
futuro pode escolher formato apropriado sem mudar a semântica, desde que a identidade seja
persistida/configurada e não regenerada em cada startup.

A implementação deve **rejeitar** whitespace acidental ao redor da identidade em vez de
normalizá-lo silenciosamente, para não criar aliases de uma chave durável.

### 4. Escopo de unicidade e não reutilização

Dentro do mesmo conjunto de dados/instalação Limiar, uma identidade não pode representar
duas acquisitions semanticamente diferentes.

Uma identidade aposentada não é reciclada para uma aquisição não relacionada. Restore,
importação ou composição de configurações que produziria colisão deve falhar ou exigir
mapeamento explícito; não há auto-renomeação silenciosa.

### 5. Lifecycle e mudanças de configuração

A identidade sobrevive a mudanças **operacionais** que não alteram a semântica de
admissão, por exemplo ajustes de concorrência, timeout, observabilidade ou labels de
operação.

Enquanto o envelope físico de Evidence não carregar uma versão separada da configuração,
mudanças que podem alterar materialmente **quais observações pertencem ao scope** exigem
uma nova `subscription_id`. Isso inclui, por padrão:

- adicionar/remover fontes do scope;
- mudar filtros/regras que alterem o conjunto admitido;
- trocar o vínculo de autorização Telegram quando isso puder alterar o universo observável;
- redefinir o propósito semântico da aquisição.

Esta regra preserva proveniência histórica sem exigir agora um `subscription_version`
adicional no schema de Evidence. Uma Decision futura pode introduzir versionamento
explícito e então revisar esta política.

O primeiro lifecycle produtivo pode carregar uma snapshot imutável da configuração por
execução; hot reload não é requisito do MVP. Uma implementação futura de hot reload deve
preservar as mesmas regras de identidade e transição.

### 6. Cardinalidade: modelo plural, MVP pode começar com uma subscription

A arquitetura permite múltiplas Acquisition Subscriptions. O MVP pode iniciar com uma
única subscription ativa por composição se isso bastar ao caso real, mas:

- o ID continua explícito;
- não existe singleton global implícito `"telegram"` ou equivalente;
- código durável não depende de existir exatamente uma subscription para sempre.

Não é necessário criar agora UI administrativa, registry distribuído ou framework
multi-tenant.

### 7. Sobreposição entre subscriptions é explícita

Se uma observação for classificada como pertencente a mais de uma subscription, cada
admissão possui sua própria `subscription_id` e produz Evidence correspondente.

Antes de encaminhar uma observação para uma authority que possa certificar progresso,
todas as Evidence exigidas pelas subscriptions aplicáveis devem estar duráveis. Se um
append falhar depois de outro já ter sido persistido:

- não ocorre forward para a transição protegida;
- a Evidence já persistida permanece;
- retry/replay pode produzir nova Evidence;
- exactly-once não é prometido.

Nenhuma deduplicação por mensagem/hash pode apagar essa distinção de provenance.

A política concreta que decide quais subscriptions se aplicam a um envelope é
responsabilidade de configuração/classificação e deve possuir testes próprios; esta
Decision não declara que todo update Telegram é comercialmente relevante.

### 8. Relação com Evidence, sync e backfill

`subscription_id` é metadata de correlação entre authorities, não fusão delas.

- Evidence: registra sob qual aquisição a observação foi admitida;
- `SourceSyncState`: quando sua Decision/schema forem aceitos, particiona continuidade
  live pela acquisition aplicável;
- `BackfillProgress`: quando sua Decision/schema forem aceitos, correlaciona cobertura
  histórica com a acquisition correspondente.

Evidence, live sync e backfill continuam com lifecycle e autoridade próprios. Compartilhar
a chave não autoriza um deles a substituir o outro.

### 9. Relação com Telegram authorization e MCP

`TelegramAuthorizationIdentity` identifica uma autorização Telegram e pode servir uma ou
mais acquisitions. Ela não substitui `subscription_id`.

Target MCP delimita o que o ChatGPT pode consultar em realtime. Ele não é Acquisition
Subscription e não vira `subscription_id` mesmo quando, por coincidência, aponta para a
mesma fonte.

O mesmo canal pode existir simultaneamente como target MCP e fonte de aquisição sem que
essas configurações compartilhem identidade ou authority.

### 10. Migração do legado

A lista histórica de canais monitorados pode ser insumo para criar uma nova Acquisition
Subscription, mas a transição precisa atribuir uma identidade explícita ao novo scope.

É proibido converter automaticamente cada `channel_id` em `subscription_id` apenas para
facilitar migração. Mapeamentos de migração devem ser explícitos e auditáveis.

### 11. Observabilidade

Como `subscription_id` não é segredo, ele pode participar de logs/traces operacionais
quando útil. Uso como label de métrica exige cardinalidade configurada/bounded e não deve
ser adotado automaticamente.

Payload Telegram, credenciais e demais dados sensíveis continuam fora de logs por default.

## Opções consideradas

### A. `channel_id` como identidade

Rejeitada. Mistura fonte e configuração, impede representar naturalmente scopes
multi-fonte e transforma uma conveniência do Telegram em identidade durável do Limiar.

### B. `TelegramAuthorizationIdentity` ou session ID

Rejeitada. A autorização é credential/lifecycle boundary e pode servir múltiplas
acquisitions; session ID é detalhe MTProto.

### C. target MCP como identidade

Rejeitada. Read scope MCP e acquisition durability são authorities distintas e detachable.

### D. ID gerado em cada startup

Rejeitada. Quebra correlação histórica, restart/replay e futuras authorities de progresso.

### E. constante global implícita enquanto existir uma única subscription

Rejeitada. Otimiza o primeiro deployment às custas de criar uma identidade artificial e
um singleton difícil de remover.

### F. hash automático da configuração

Rejeitada. Faz mudanças incidentais reidentificarem a aquisição, acopla identidade à
serialização e torna evolução/migração frágeis.

### G. manter o mesmo ID após mudanças semânticas sem registrar versão

Rejeitada no schema atual. A Evidence histórica perderia a capacidade de distinguir qual
scope efetivamente autorizou a admissão.

## Gates de implementação

Antes de chamar o wiring produtivo de Source Admission de validado:

1. configuração/composition root fornece `subscription_id` explícito antes de instalar o
   ingress;
2. vazio, whitespace, duplicidade/ambiguidade falham antes da admissão;
3. restart com a mesma configuração preserva exatamente a mesma identidade;
4. nenhum caminho deriva ID de canal, mensagem, peer, autorização Telegram, sessão ou MCP;
5. Evidence persistida contém exatamente a identidade configurada;
6. append failure continua impedindo forward;
7. teste cobre uma observação aplicada a mais de uma subscription, incluindo falha parcial
   de append e replay seguro;
8. mudanças semanticamente relevantes de scope exigem nova identidade enquanto não houver
   versionamento de configuração na Evidence;
9. configuração/adapter não registra segredo dentro da identidade;
10. migração de canais legados, quando implementada, exige mapping explícito;
11. integração real continua sujeita aos gates de crash/restart, recovery e state dos ADRs
    correspondentes.

## Fora do escopo

Esta Decision não define:

- formato UUID/ULID/slug definitivo;
- UI/CLI final de gerenciamento;
- tabela/arquivo/provider definitivo de configuração;
- hot reload;
- schema de `SourceSyncState`;
- schema de `BackfillProgress`;
- política completa de classificação/filtro de eventos;
- cardinalidade máxima de subscriptions;
- multi-tenancy;
- migração do banco legado;
- Telegram real, OTP/2FA, Secure MCP Tunnel, merge ou deploy.

## Consequências

A identidade de aquisição deixa de ser um placeholder e passa a ser uma authority
configurada, estável e auditável. Isso destrava o transporte correto até Source Admission
sem confundir canal, sessão ou MCP.

A regra de nova identidade para mudanças semânticas evita adicionar agora versionamento de
configuração ao schema de Evidence, reduzindo complexidade do MVP sem sacrificar
proveniência. O custo é exigir transição explícita quando o scope de aquisição mudar.

Suporte conceitual a múltiplas subscriptions aumenta a correção futura sem exigir que o
primeiro deployment implemente orchestration multi-subscription completa.

## Relação com decisões e Evidence anteriores

- ADRs históricos 016–020 continuam herdados para L3-004 conforme documentação L3;
- o ADR histórico 024 permanece preservado como Proposal/Evidence histórica e **não** é
  promovido retroativamente;
- esta L3 ADR 006 é a authority corrente de identidade de Acquisition Subscription no
  Limiar 3;
- L3 ADR 002 continua authority de Telegram authorization/runtime;
- L3 ADR 004 continua authority do MCP realtime;
- ADRs históricos 021/022 permanecem Proposed e não são aceitos por consequência.

## Acceptance

O mantenedor resolveu explicitamente a identidade de Acquisition Subscription em
2026-10-01 e, em seguida, esclareceu que a Decision poderia ser completa/avançada por se
tratar de um boundary importante. O registro durável dessa autorização está em
`docs/limiar3/DECISION_ACCEPTANCE_2026-10-01_SUBSCRIPTION_IDENTITY.md`, com
`Acceptance-reference: 0ce7d88f2b47db397ae6463d9857c5ca9212e013`.
