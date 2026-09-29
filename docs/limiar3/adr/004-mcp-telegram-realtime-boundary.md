# L3 ADR 004 — MCP Telegram realtime: SDK oficial, transporte privado e read scope

Authority: Decision Record — Limiar 3.0
Status: Proposed

## Contexto

L3-003 precisa expor a `TelegramQuery` read-only para ChatGPT cedo, antes do modelo comercial, sem transformar MCP em segunda ingestão, sem entregar a credencial Telegram ao adapter e sem tornar consulta realtime em Source Evidence.

A superfície MCP é uma boundary estrutural: introduz protocolo/dependência externa, transporte e uma authority de autorização do consumidor distinta da `TelegramAuthorizationIdentity`. Portanto não deve nascer de defaults implícitos nem ser tratada como simples detalhe de handler.

### Evidence externa verificada em 2026-09-24 e revalidada em 2026-09-29

- `github.com/modelcontextprotocol/go-sdk v1.8.0` é a latest stable upstream observada, publicada em 2026-09-14. O módulo declara Go 1.25.0, compatível com o baseline Limiar Go 1.27.1.
- v1.8.0 negocia MCP `2026-07-28` como versão mais nova e inclui hardening de transportes contra resource exhaustion, session leaks, deadlocks e teardown hangs.
- MCP `2026-07-28` adota core stateless/sessionless. No Go SDK, Streamable HTTP serve essa revisão com `StreamableHTTPOptions.Stateless = true`.
- Na tag v1.8.0, `StreamableHTTPOptions` mantém proteção de localhost/DNS rebinding habilitada por default, limita request body a 4 MiB por default e exige opt-in explícito de `PropagateRequestCancellation` para vincular o handler ao lifecycle do request HTTP em `2026-07-28`.
- Na mesma tag, `ServerOptions.Capabilities == nil` preserva por compatibilidade uma capability `logging` default; como logging está deprecated em MCP `2026-07-28`, a primeira superfície Limiar deve anunciar capabilities mínimas explicitamente em vez de herdar esse default histórico.
- O SDK oficial fornece `mcp.NewServer`, `mcp.AddTool` tipado e `mcp.NewStreamableHTTPHandler`; não há motivo para o Limiar reimplementar JSON-RPC, SSE ou schema generation.
- A documentação atual da OpenAI permite conectar MCP privado a ChatGPT/Codex/API através de Secure MCP Tunnel. O `tunnel-client` inicia conexão HTTPS outbound e pode encaminhar para MCP local por HTTP ou stdio; o servidor privado não precisa ganhar listener público.
- Permissões de tunnel e acesso a developer mode são authorities externas e separadas. Esta Proposal **não assume** que o mantenedor ou um workspace específico já possua essas permissões.

Esses fatos externos devem ser revalidados na integração real; não são garantia perpétua de produto/plano.

## Proposal

### 1. SDK e protocolo

A primeira implementação de L3-003 usa exatamente:

`github.com/modelcontextprotocol/go-sdk v1.8.0`

sob a política `Current Stable First`.

O Limiar usa as abstrações do SDK oficial para server, tools, JSON schema e Streamable HTTP. Não cria protocolo MCP próprio, JSON-RPC próprio, SSE próprio nem wrapper genérico sobre o SDK.

O servidor opera em **Streamable HTTP stateless**. A implementação não restringe artificialmente `SupportedProtocolVersions` sem incompatibilidade demonstrada; deixa o SDK negociar o conjunto suportado, tendo `2026-07-28` como revisão corrente.

### 2. Private by default

A primeira topologia é single-host, single-operator e **loopback-only**.

- o listener HTTP inicial faz bind somente em loopback (`127.0.0.1`/`::1`) e configuração non-loopback falha fechado;
- `0.0.0.0`, endereço de LAN, reverse proxy ou ingress externo ficam fora da primeira fatia e exigem Decision proporcional sobre autenticação, trust boundary e deployment;
- o host/usuário do SO é parte explícita do trust model: loopback reduz reachability, mas não autentica outros processos locais; host multiusuário ou não confiável fica fora do contrato inicial;
- nenhum endpoint público é requisito de L3-003;
- nenhum listener público sem nova Decision de segurança/deployment;
- para ChatGPT/Codex/API alcançar o servidor privado, Secure MCP Tunnel é o caminho operacional suportado verificado nesta revisão; `tunnel-client` deve rodar na mesma trust boundary que já consegue alcançar o MCP local;
- `tunnel-client`, `tunnel_id`, runtime API key e permissões OpenAI não pertencem ao core do Limiar e não são gerenciados pelo Telegram boundary.

Tunnel é transporte/reachability; não é identidade Telegram nem substituto da autorização de dados.

### 2.1 Hardening HTTP mínimo da primeira fatia

A implementação inicial deve usar os primitives do SDK/stdlib sem enfraquecer defaults de segurança:

- `StreamableHTTPOptions.Stateless = true`;
- `StreamableHTTPOptions.PropagateRequestCancellation = true` para que cancelamento do request moderno alcance a tool e a `TelegramQuery`;
- não desabilitar a proteção de localhost/DNS rebinding do SDK;
- manter request body explicitamente bounded; o default de 4 MiB do SDK é limite aceitável inicial e pode ser reduzido sem novo ADR;
- envolver o handler MCP com `http.CrossOriginProtection` da stdlib, sem liberar origins arbitrárias por default;
- criar o `mcp.Server` com capabilities explícitas mínimas, evitando a capability `logging` histórica que o SDK anuncia quando `Capabilities` é nil;
- não habilitar roots, sampling, prompts, resources ou outras capabilities sem necessidade do slice.

Esse hardening protege a superfície local sem introduzir auth server, proxy próprio ou stack de transporte paralela.

### 3. Authorization/read scope separado da credencial Telegram

A primeira superfície é **single-operator/private-development**, read-only e explicitamente scoped.

O MCP adapter recebe:

1. uma `TelegramQuery` já composta;
2. uma configuração de **targets nomeados** autorizados, conceitualmente `target_name -> PeerRef`.

A ferramenta aceita `target_name`, não aceita `PeerRef` arbitrário, `PeerKey` arbitrário, access hash, session bytes, phone/OTP/password ou `tg.*`.

Esse scope:

- pertence ao MCP realtime adapter;
- não é `subscription_id` de aquisição;
- não é Evidence authority;
- não cria identidade comercial;
- pode ser revogado removendo/desabilitando o target na configuração operacional.

A conta Telegram continua definindo o máximo tecnicamente acessível; o read scope MCP reduz esse máximo para o conjunto explicitamente autorizado.

### 4. Primeira tool

A primeira tool deve ser uma única operação de history read-only, conceitualmente:

`telegram.history(target, limit, cursor)`

O nome final pode ser ajustado sem novo ADR se a semântica não mudar.

Fluxo:

```text
MCP call
  -> validate target against configured read scope
  -> TelegramQuery.ResolvePeer(configured PeerRef)
  -> TelegramQuery.History(PeerKey, bounded request)
  -> map safe DTO
  -> MCP structured result
```

Chamar `ResolvePeer` no fluxo mantém o adapter robusto a eviction do peer cache memory-only sem criar peer database no MCP.

O resultado identifica explicitamente que é **Telegram realtime observation / not Source Evidence**. O adapter não grava SQLite, não chama Source Admission e não faz side effect Telegram.

### 5. Mensagens são dados não confiáveis

Texto, links e metadata vindos do Telegram são payload de dados não confiáveis.

O MCP adapter:

- não executa comandos presentes em mensagens;
- não segue URL automaticamente;
- não interpreta texto da mensagem como instrução de sistema;
- não injeta payload Telegram em logs/erros operacionais;
- entrega conteúdo somente como dados estruturados ao consumidor.

### 6. Auth do consumidor na primeira fatia

O Limiar **não implementa um OAuth server próprio em L3-003 inicial**.

Justificativa: a primeira superfície é privada, single-operator, read-only, loopback-only e com scope fixo de targets. O trust model inicial depende do usuário/host local confiável; loopback não é reinterpretado como autenticação multiusuário. Quando conectada a ChatGPT, a reachability usa Secure MCP Tunnel e as permissões do produto/workspace; essas permissões não são tratadas como credencial Telegram.

Se surgir qualquer um destes requisitos, esta parte deve ser revisada por Decision proporcional antes de exposição:

- endpoint MCP público;
- app publicada/compartilhada;
- múltiplos usuários com scopes distintos;
- identidade/per-user audit;
- write actions;
- delegation/OAuth próprio do Limiar.

### 7. Failure domain

Falha de uma tool MCP não altera session storage, Telegram authorization, collector, Evidence ou estado comercial.

O MCP adapter não recebe ownership do `telegram.Client`; recebe somente `TelegramQuery`.

Falha fatal do listener/transport pertence ao lifecycle do adapter MCP. A política do composition root deve permitir que esse componente seja reiniciado/desabilitado sem redefinir authority do Telegram runtime; não criar supervisor/framework distribuído antecipadamente.

## Gates de implementação

Antes de marcar a primeira fatia L3-003 como validada:

1. pin exato do SDK oficial + `go.sum`;
2. build/vet/test/race/govulncheck no toolchain aceito quando o canal de execução existir;
3. teste do server/tool com client MCP real ou harness oficial, não apenas chamada direta de handler;
4. Streamable HTTP com `Stateless: true` e `PropagateRequestCancellation: true`;
5. listener inicial limitado a loopback; configuração non-loopback rejeitada;
6. proteção de localhost/DNS rebinding não desabilitada, `http.CrossOriginProtection` aplicada e request body bounded;
7. capabilities explícitas mínimas: tool read-only esperada, sem `logging` default, roots, sampling, prompts ou resources;
8. somente tools read-only esperadas aparecem em discovery/list;
9. target ausente/desabilitado falha antes de invocar `TelegramQuery`;
10. input não permite peer arbitrário nem access hash;
11. limit/cursor/cancellation preservados end-to-end;
12. erros Telegram são traduzidos sem vazar raw error/payload;
13. output não contém session bytes, OTP, password, API hash, access hash ou tipos `tg.*`;
14. mensagens continuam marcadas semanticamente como realtime/not-Evidence;
15. nenhum import/acesso a SQLite/Evidence no MCP realtime adapter;
16. teste de payload hostil confirma que conteúdo Telegram não vira comando do servidor;
17. teste de request cross-origin hostil confirma rejeição antes da tool;
18. teste de teardown/cancelamento do HTTP adapter;
19. integração ChatGPT/tunnel somente quando houver autorização/permissões reais; não assumir `tunnel_id`, developer mode ou credenciais.

## Fora do escopo

Esta Proposal não decide:

- MCP Limiar-data sobre Query Service;
- OAuth público/per-user;
- publicação de app;
- servidor multi-tenant;
- write tools;
- Source Admission;
- collector/recovery/backfill;
- storage de conversas;
- tool de mídia;
- exposição pública/Internet ingress;
- criação/configuração real de Secure MCP Tunnel.

## Consequências

A primeira integração MCP fica pequena e substituível: protocolo/transport ficam no SDK oficial; autorização de dados fica num read scope explícito; Telegram continua atrás de `TelegramQuery`; e o caminho para ChatGPT pode permanecer privado.

Há uma nova dependência de produto e uma nova configuração de scope, mas não nasce uma segunda stack de rede, auth Telegram ou persistence layer.

## Acceptance Gate

Enquanto este ADR estiver `Proposed`:

- não adicionar `modelcontextprotocol/go-sdk` ao root;
- não implementar servidor/tool MCP de produto;
- não criar tunnel, app, OAuth ou endpoint público.

O mantenedor pode aceitar, revisar ou rejeitar esta Proposal. Só `Accepted` autoriza a implementação estrutural descrita aqui.
