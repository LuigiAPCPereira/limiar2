# SESSION_LOG — Limiar 3.0

**Origem:** síntese de afirmações do mantenedor e verificações GitHub da conversa de 2026-09-22; não inventa decisões ou fatos de outros chats. Logs anteriores e PRs permanecem nas fontes originais.

## Conhecimento prévio da rebaseline, a não perder

- O Limiar 1 foi interrompido devido a forte acoplamento e problemas funcionais, incluindo imagens não coletadas corretamente segundo o mantenedor. Causa do bug não reproduzida aqui.
- Rebaseline arquitetural já foi além de documentos: primeiro SQLite/Evidence lado a lado implementado, com guards; ADRs 016–020 Accepted. PR #211 integrada para rejeitar schema incompleto/futuro. Isso não é substituição completa de Tursogo.
- Sync/backfill físicos seguem ADRs 021/022 Proposed, sem aprovação; session storage ADR 023 Proposed, identidade de aquisição ADR 024 Proposed. `EXP-LIMIAR-019` Unix/intra-processo Supported e PR #202 integrada em 2026-09-14, mas apenas teste/experimento, não runtime. EXP-007 em banco histórico real pendente. Portabilidade macOS inconclusiva na narrativa fornecida. Separação sessão vs peer cache registrada em F-STO-006.
- PR documental de adoção v2 #214 está draft e **ADOÇÃO PARCIAL**. Seu HEAD antes desta branch foi `7177d929839512b8005ae379d96e3d224feac1f8`; base main `796b7769449b72320b27c2557c2ccb8c8eb183e1`. A branch Limiar 3.0 foi criada a partir daquele HEAD, para herdar AGENTS/protocolo sem alterar a main.

## Evolução da direção solicitada pelo mantenedor

1. Inicialmente pediu reconstruir 100% a **fundação de ingestão** bottom-up começando autenticação/sessão, mensagens, coleta e imagens, com separação clara, Engineering DNA, segurança, legibilidade, robustez e performance demonstrada. Essa diretriz foi registrada em `docs/REBASELINE_SCOPE.md` no PR #214.
2. Enviou proposta mais ampla: avaliar arquitetura **nova para toda a implementação Limiar**, em ambiente isolado no mesmo repo, aproveitando conhecimento e capacidades validadas, preservando dados/comportamentos relevantes e reescrevendo incrementalmente sem transposição literal do legado. Pediu avaliar ANTES de branch/implementação, sem decisões técnicas precipitadas.
3. Propôs MCP Telegram também para a própria conversa com ChatGPT, para perguntar diretamente sobre promoções e acessar mensagens autorizadas, além de apoio ao desenvolvimento. Isso expande o produto, não cria um segundo coletor.
4. Explicitou que **Limiar e MCP devem ser construídos juntos** e cogitou reutilizar a autenticação do Limiar; ficou explicitada a necessidade de distinguir sessão MTProto do coletor e eventual autorização de consumidor MCP. Identidade de conta de usuário Limiar não comprovada no produto anterior.
5. Corrigiu o assistente: **não existe app Limiar extra**. O produto é coleta → MCP integrado em paralelo → processamento → dados limpos → API → frontend de agrupador de promoções. MCP não precisa estar no caminho crítico, mas deve constar desde a fundação e dar consulta a mensagens e ofertas.
6. Reafirmou que **não havia autorizado implementação**: antes seria feita avaliação arquitetural. Em seguida, neste pedido, autorizou especificamente **criar uma NOVA branch e colocar somente documentação**: Engineering DNA, Agent Development Protocol como projeto novo, e contexto completo Limiar 3.0; objetivo é outro chat investigar projeto inicial + autenticação/sessão. Isso NÃO autoriza código, auth real, Telegram, MCP conectado, merge ou deploy.

## Ações documentais desta execução

- Confirmado GitHub `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`, PR #214 ainda draft HEAD `7177d929839512b8005ae379d96e3d224feac1f8`, repositório privado com permissão reportada de push.
- Criada branch `docs/limiar-3-foundation-20260922` diretamente do HEAD documental, sem merge.
- Criados documentos de entrada, produto, arquitetura, tracker, checkpoint, relatório de inicialização e guia derivado do Engineering DNA. Reabertura final da ref e inclusão integral do DNA devem determinar se L3-DOC-001 estará validada ou parcial.
- Nenhuma decisão de mecanismo de sessão, arquivo de configuração, estrutura de pacotes, OAuth/MCP ou schema foi aceita nesta escrita.

**Próximo capítulo:** outro chat deve recuperar START_HERE e executar somente a investigação `L3-001` (projeto inicial + autenticação/sessão) até receber autorização de engenharia específica. Registrar novas decisões e experimentos no futuro, sem retroagir status de fontes.

## Consolidação da rebaseline para Limiar 3 — L3-DOC-002

A pedido explícito do mantenedor, o conhecimento arquitetural da Rebaseline 2026 foi consolidado em contexto recuperável do Limiar 3, sem criar nova Decision. Foram criados `REBASELINE_INHERITANCE.md` e `BOTTOM_UP_REBUILD_PLAN.md`.

A regra registrada é: Limiar 3 herda invariantes, contratos aceitos, Evidence e limites; não herda automaticamente mecanismos experimentais, schemas Proposed, topologia física ou packages do legado. O plano bottom-up separa sessão, Evidence, subscription/admission, live recovery, backfill, peer cache, mídia, projections, processing, modelo comercial, query e superfícies MCP/API/frontend, deixando migração/cutover por último.

A escrita foi exclusivamente documental. ADRs 021–024 permanecem Proposed e `L3-001` continua sendo a próxima investigação funcional.
