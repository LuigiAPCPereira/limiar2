# L3-001C — auditoria upstream-only do gotd/td

**Estado:** concluída; resultado em [L3_001C_GOTD_UPSTREAM_AUDIT.md](L3_001C_GOTD_UPSTREAM_AUDIT.md).  
**Tipo:** pesquisa externa de dependência estrutural.  
**Não autoriza código, upgrade, login Telegram ou mudança de ADR.**

## Por que esta pesquisa existe

L3-001A e L3-001B pesquisaram extensivamente MTProto e gotd, inclusive documentação e código upstream, mas sempre para responder perguntas do Limiar.

Falta uma investigação deliberadamente invertida:

> estudar o gotd como biblioteca/produto por si só, sem partir da arquitetura atual do Limiar, e somente no final mapear descobertas úteis para L3-002.

Essa auditoria é limitada. Não deve repetir MTProto inteiro nem refazer L3-001A.

## Pergunta central

> O que o gotd stable atual realmente oferece, quais são seus boundaries, extensões, garantias, riscos, performance characteristics e armadilhas conhecidas, e qual é a forma idiomática de estendê-lo sem duplicar ou lutar contra sua arquitetura?

## Escopo upstream-only

Usar apenas fontes externas durante a coleta inicial:

1. repositório oficial gotd/td;
2. release/tag stable atual;
3. changelog/releases entre v0.161.0 e stable;
4. ARCHITECTURE.md e ROADMAP.md;
5. README/docs/examples oficiais;
6. package docs/código da stable atual;
7. gotd/contrib e subprojetos relevantes;
8. issues/PRs abertas e recentes;
9. security policy/advisories;
10. benchmarks upstream e metodologia;
11. CI/fuzz/e2e/canary strategy;
12. projetos públicos maduros que usam gotd em produção.

Somente após fechar esse inventário, comparar com necessidades de L3-002.

## Inventário obrigatório

Mapear:

- telegram.Client;
- Options;
- auth;
- qrlogin;
- session;
- peers;
- query;
- updates;
- downloader;
- uploader;
- message;
- tgerr;
- pool;
- mtproto;
- transport;
- middleware;
- tracing;
- floodwait/rate limiting;
- gotd/contrib;
- CLI/examples;
- testing server/e2e/fuzz/canary.

Para cada componente:

| Componente | Stable/maturidade | API/ownership | Performance/lifecycle | Pitfalls/issues | Relevância potencial |
| --- | --- | --- | --- | --- | --- |

## Stable atual vs v0.161.0

Produzir diff conceitual:

- Telegram layer/schema;
- auth;
- session persistence;
- reconnect;
- migration;
- pools;
- updates;
- peer helpers;
- query;
- media;
- concurrency;
- middleware;
- observability;
- security;
- dependencies.

Classificar diferenças:

- fix importante;
- feature relevante;
- irrelevante ao Limiar;
- risco/regressão;
- exige experimento.

## Extension points

Responder claramente:

- quais interfaces foram feitas para implementação externa;
- quais packages são intencionalmente high-level;
- quais são experimentais/WIP;
- o que o upstream espera que aplicações personalizem;
- onde wrappers costumam ser úteis;
- onde wrappers apenas duplicam o SDK;
- qual a forma idiomática de session.Storage;
- como inserir logging/tracing/middleware;
- como lidar com auth bootstrap;
- como compartilhar um client;
- como controlar concurrency;
- como fazer query/pagination;
- como conectar updates.Manager;
- como estender media.

## Performance

Pesquisar Evidence real disponível:

- memory footprint;
- allocation strategy;
- pooling;
- request multiplexing;
- connection pools;
- concurrency;
- reflection/codegen;
- benchmarks;
- canary/performance tests;
- known scaling limits.

Separar marketing/claim upstream de benchmark independente.

Não criar números de capacidade para o Limiar.

## Segurança

Investigar:

- security policy;
- audit status;
- crypto implementation boundaries;
- key/session handling;
- RNG;
- replay protections;
- PFS;
- known vulnerabilities;
- advisories;
- unsafe usage;
- logging risks;
- auth/session threat guidance.

## Issues atuais

Analisar issues recentes relevantes, incluindo quando ainda abertas:

- deadlocks/middleware/reentrancy;
- updates.Manager scale;
- media/upload/download;
- session/auth;
- reconnect/DC migration;
- goroutine/memory leak;
- flood handling.

Issue aberta é Evidence de risco/hipótese, não prova de que o Limiar sofrerá o bug.

## Produção real

Pesquisar usuários/projetos públicos do gotd:

- arquitetura;
- lifecycle;
- auth/session;
- peer storage;
- updates;
- media;
- shutdown;
- observability.

Não copiar patterns sem entender contexto.

## Entrega

1. resumo da biblioteca;
2. arquitetura upstream;
3. capability map;
4. maturity map;
5. stable atual;
6. v0.161.0 → stable diff;
7. extension points;
8. auth/session patterns;
9. concurrency/runtime;
10. updates;
11. peers/query;
12. media;
13. middleware/retry/flood;
14. observability;
15. performance;
16. security;
17. testing strategy upstream;
18. known issues;
19. production usage;
20. recommendation de versão para L3-002;
21. “use diretamente / adapte / evite / investigue”;
22. unknowns que realmente exigem experimento.

## Stop condition

A pesquisa termina quando for possível responder:

> Qual versão stable do gotd devemos pinar para L3-002, quais capabilities devemos usar diretamente e quais poucas extensões o Limiar realmente precisa implementar?

Não reabrir decisões de produto, Evidence DB, modelo comercial, MCP protocol ou frontend.

Nenhum código deve ser implementado ao final.


## Fechamento

A stop condition foi atingida. A auditoria recomenda `gotd/td v0.162.0` como pin para L3-002, sujeito à decisão do mantenedor e Implementation Gate; reduz a extensão L3 a hardened session storage, runtime ownership/readiness, query adapter, error translation e bounded admission. Não recomenda nova pesquisa ampla antes de L3-002.
