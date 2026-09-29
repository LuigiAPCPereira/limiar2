# Registro de aceitação — L3 ADR 004 / MCP Telegram realtime

**Data local do mantenedor:** 2026-09-29  
**Escopo:** Limiar 3.0 — L3-003 / MCP Telegram realtime.  
**Decision aceita:** `docs/limiar3/adr/004-mcp-telegram-realtime-boundary.md`, incluindo a correção posterior de responsabilidade entre ChatGPT e MCP registrada neste mesmo artefato.

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

3. **Busca e análise ficam no ChatGPT, não no MCP**
   - perguntas como "procure Samsung e compare os menores preços" são dirigidas ao ChatGPT;
   - o MCP fornece as operações read-only necessárias para o ChatGPT listar targets, obter informações, ler/paginar históricos e recuperar mensagens;
   - o ChatGPT percorre o corpus autorizado e faz busca textual/semântica, filtragem, interpretação de preço, comparação e resumo;
   - esta aceitação **não exige uma tool `telegram.search` nem uma engine de busca/comparação comercial dentro do MCP**;
   - a análise do ChatGPT não se transforma em Evidence/Finding canônico automaticamente.

4. **Perguntas cotidianas pelo ChatGPT**
   - casos de uso aceitos incluem procurar produtos, comparar valores aparentes, recuperar a mensagem original e responder perguntas com base no corpus Telegram acessível via MCP;
   - o MCP Telegram realtime é parte da experiência normal do produto, além de servir à descoberta/modelagem durante o desenvolvimento.

5. **Uso recorrente / monitoramento**
   - solicitações como "ficar de olho em promoções de celular" podem ser atendidas por chamadas repetidas/polling do ChatGPT/consumidor autorizado sobre as operações read-only quando o ambiente de automação conseguir acessar o MCP;
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

A correção posterior do mantenedor tornou explícito que busca/filtragem/comparação pertencem ao ChatGPT. O desenho aceito não depende de uma tool MCP de busca semântica/comercial: o MCP fornece acesso read-only fiel e paginável; o consumidor realiza o raciocínio da pergunta.

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
