# Registro de aceitação — L3 ADR 004 / MCP Telegram realtime

**Data local do mantenedor:** 2026-09-29  
**Escopo:** Limiar 3.0 — L3-003 / MCP Telegram realtime.  
**Decision candidata aceita:** `docs/limiar3/adr/004-mcp-telegram-realtime-boundary.md` na revisão `eb432bc34a88c84dbbd25e1fd1424305203b37b2`.

## Aceitação do mantenedor

Na conversa do projeto, o mantenedor aprovou explicitamente o L3 ADR 004 e ampliou a intenção funcional do MCP Telegram realtime.

A aceitação cobre a Proposal endurecida de transporte/read scope e estabelece que o MCP é uma superfície de produto para o ChatGPT, não apenas uma ferramenta de desenvolvimento.

## Escopo aceito

1. **Visão read-only dos canais cadastrados**
   - o ChatGPT pode consultar os targets/canais explicitamente cadastrados e autorizados;
   - pode obter histórico, mensagens e informações/metadata do canal;
   - pode obter conteúdo e metadata de mensagem com fidelidade suficiente para analisar o corpus.

2. **"Telegram cru" com boundary seguro**
   - "cru" significa representação semanticamente fiel e pouco transformada do que o Telegram fornece;
   - texto/caption, entities, IDs, timestamps, edição, relações de reply/forward/thread/group, metadata de mídia, contadores/flags e outros campos úteis podem ser preservados quando disponíveis;
   - isso **não** autoriza expor session bytes, auth key, OTP/password, API hash, access hashes, `tg.*`, `InputPeer` ou internals MTProto ao consumidor MCP.

3. **Busca nos canais cadastrados**
   - ChatGPT deve poder buscar termos como "Samsung" dentro do conjunto de targets autorizados;
   - busca multi-target permanece limitada aos canais cadastrados e não ganha acesso global ao Telegram por conveniência;
   - interpretação de preço, comparação e resumo podem ser feitos pelo ChatGPT sobre as mensagens encontradas, sem transformar essa inferência em Evidence/Finding canônico automaticamente.

4. **Perguntas cotidianas pelo ChatGPT**
   - casos de uso aceitos incluem procurar produtos, comparar valores aparentes, recuperar a mensagem original e responder perguntas com base no corpus Telegram acessível via MCP;
   - o MCP Telegram realtime é parte da experiência normal do produto, além de servir à descoberta/modelagem durante o desenvolvimento.

5. **Uso recorrente / monitoramento**
   - solicitações como "ficar de olho em promoções de celular" podem ser atendidas por chamadas repetidas/polling do consumidor autorizado quando o ambiente ChatGPT/automação conseguir acessar o MCP;
   - o MCP stateless não ganha scheduler, watch state ou alerting persistente server-side por implicação;
   - watch state server-side futuro exige Decision proporcional porque adiciona estado/lifecycle próprio.

6. **Mídia**
   - metadata e referências de mídia fazem parte da visão Telegram;
   - leitura/download binário de mídia pode entrar em slice read-only posterior sob o mesmo boundary, com gates próprios de segurança, tamanho e lifecycle;
   - não é requisito para o primeiro tracer slice.

## Fundamentos técnicos associados

A revisão aceita mantém:

- `github.com/modelcontextprotocol/go-sdk v1.8.0`;
- Streamable HTTP stateless e loopback-only na primeira topologia;
- hardening HTTP registrado no ADR;
- targets nomeados como read scope;
- `TelegramQuery`/capability como boundary em vez de ownership direto do `telegram.Client`;
- ausência de SQLite/Evidence no MCP realtime;
- ausência de write tools e OAuth próprio na primeira fatia.

A auditoria do `gotd/td v0.162.0` confirmou suporte de busca por peer via `telegram/query/messages.Search(peer)`, com query textual, datas, filtros e paginação. O desenho aceito usa essa direção por target autorizado em vez de busca global irrestrita.

## Limites da aceitação

Esta aceitação autoriza a **Decision arquitetural** e libera a implementação de L3-003 dentro do boundary aceito, mas não autoriza por si só:

- usar conta/credencial Telegram real;
- solicitar OTP/2FA;
- criar/revogar autorização Telegram;
- criar/configurar Secure MCP Tunnel real;
- expor listener público/non-loopback;
- criar OAuth, multi-user ou write tools;
- fazer merge/deploy;
- tratar respostas realtime como Source Evidence automaticamente.

Essas ações continuam sujeitas aos gates e autorizações correspondentes.

## Referência

Este registro foi criado para fornecer a referência durável exigida por `AGENTS.md` para a promoção `Proposed -> Accepted` do L3 ADR 004.
