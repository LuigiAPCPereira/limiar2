# Limiar 3.0 — visão, escopo e critérios iniciais

**Natureza:** requisitos expressos pelo mantenedor nos diálogos de 2026-09-22 e neste pedido; não implica implementação/aceitação automática de mecanismos técnicos. Esta é a fonte de escopo **da frente Limiar 3.0** nesta branch. `docs/REBASELINE_SCOPE.md` registra escopo anterior (reconstrução integral da fundação de ingestão) e permanece preservado como história. Não confundir versões ou declarar que foi atualizado na `main`.

## Produto e problema

Limiar é um agrupador de promoções: monitora fontes como canais do Telegram, coleta mensagens e imagens, preserva Evidence/proveniência, processa, extrai e agrupa informações comerciais, **apresenta dados limpos**, disponibiliza consultas via API e apresenta o resultado em um **frontend de agrupador de promoções**. O Limiar 1 foi interrompido pelo acoplamento excessivo e problemas funcionais, incluindo imagens não coletadas corretamente segundo relato do mantenedor. Não se demonstrou ainda a causa técnica dessas falhas neste plano.

**Direção do Limiar 3.0:** reconstruir **100% da implementação do produto**, não apenas a ingestão nem uma sucessão de remendos no código legado. Preservar comportamentos e dados necessários, ADRs Accepted, experimentos e testes comprovados, sem traduzir cada componente antigo literalmente ou transplantar seus acoplamentos. O legado é referência e fonte de aprendizado, não arquitetura imutável. O novo código convive de forma isolada até cumprir gates de migração e substituição; não alterar dados históricos in-place.

**MCP é parte do produto desde o planejamento**, desenvolvida em conjunto com as capacidades que a sustentam. O usuário quer perguntar **aqui no ChatGPT** por promoções diretamente (produto, preço, loja, canal, horário, link, imagens, comparação), bem como, com autorização apropriada, consultar mensagens originais e casos reais para diagnóstico da coleta. Não é só ferramenta de desenvolvimento. O MCP não substitui a coleta nem executa o processamento obrigatório, e sua indisponibilidade não bloqueia o pipeline principal. Pode ser uma superfície de acesso a mensagens admitidas e ofertas processadas sem replicar regras de negócio.

## Fluxo conceitual, não topologia definitiva

`Telegram/fontes → autenticação MTProto + coleta/admissão → Evidence durável → interpretação e processamento → dados de promoções limpos/agrupados → API → frontend`.

`MCP ↔ capacidades autorizadas de consulta do Limiar` é uma fronteira transversal, podendo consultar mensagens/Evidence e ofertas estruturadas através de contratos estreitos, conforme disponibilidade e autorização. A posição visual proposta pelo mantenedor ('coleta, MCP, processamento, dados limpos, API, frontend') indica inclusão **desde a coleta** e paralelismo, não exige que cada mensagem atravesse MCP antes de ser processada. Validar essa interpretação no desenho técnico, não torná-la um barramento obrigatório.

**Não existe 'segundo aplicativo Limiar'**; esse termo foi introduzido indevidamente pelo assistente e corrigido pelo mantenedor. As superfícies desejadas são API/frontend próprios e integração ChatGPT/MCP. Não assumir app mobile, plataforma SaaS multiusuário ou sistema de contas completo como requisito implícito.

## Objetivos obrigatórios de engenharia

1. Responsabilidade e lifecycle com owner inequívoco; alta coesão, baixo acoplamento, fronteiras entre domínio/aplicação/transporte/persistência/integrações/apresentação, composição explícita e interfaces apenas quando há motivo real. Não criar dezenas de classes e pastas para parecer sofisticado.
2. Performance, rapidez e eficiência desde a arquitetura: evitar trabalho redundante, cópias e consultas evitáveis; limites de concorrência e I/O; medir baseline e ganhos comparáveis antes de declarar melhoria. Não inventar SLO, volume, taxa de perda ou prazo de entrega garantido.
3. Longo prazo: código Go idiomático, legível, nomes expressivos, modularidade por ownership/motivo de mudança, contratos estreitos, pouca complexidade gratuita, extensão guiada por requisitos reais.
4. Resiliência: integridade, durabilidade, restart/crash, duplicação, idempotência quando necessária, tratamento de falhas parciais, timeout/cancelamento/backpressure/retry, observabilidade, segurança de segredos e isolamento de dados.
5. Testabilidade: testes unitários/contrato/integração e end-to-end nas fatias pertinentes; invariantes, imagens e cenários de falha; critérios de aceite e evidência da revisão exata para afirmar conclusão.
6. Execução incremental **de baixo para cima**, começando pela fundação/projeto inicial + autenticação e sessão, depois recebimento/admissão de mensagens/Evidence, sincronização/backfill/recovery, mídia, processamento/agrupamento, consultas MCP/API e frontend conforme dependências reais. Trabalhar MCP em paralelo a partir de capabilities úteis; não esperá-lo ficar por último por convenção nem bloquear coleta por ele.
7. Evitar investigação infinita: investigar só incerteza impeditiva; assim que Implementation Gate e autorização da etapa passarem, implementar fatia funcional completa e validar. A preparação documental de hoje não é autorização para código.

## Reaproveitamento classificado por autoridade

- **Accepted:** ADRs 016–020 para Source Evidence, recovery, Source Admission, SQLite local side-by-side, schema de Evidence; ver registry `docs/adr/README.md` para outras transições. ADR 004 preserva princípio de sessão MTProto durável, não escolha perpétua de Tursogo.
- **Implementado em parte:** `internal/storage/sqlite` com Evidence, guards/schema reabertura; PR #211 integrada; reutilização depende compatibilidade do novo wiring e revalidação, não de cópia cega.
- **Evidência experimental:** EXP-LIMIAR-011–017 durabilidade/portabilidade no escopo testado, macOS inconclusivo; EXP-LIMIAR-018 rejeita `gotd.FileStorage` as-is; EXP-LIMIAR-019 Supported para arquivo hardened Unix/intra-processo, PR #202 integrada, apenas `_test.go`, sem mecanismo produtivo ou suporte Windows demonstrado.
- **Proposto, não decidido:** ADR 021 schema SourceSyncState; ADR 022 BackfillProgress; ADR 023 armazenamento hardened da sessão; ADR 024 identidade `subscription_id`. Não colocar proposta em produção sem aceite formal; não converter fixture `telegram:test` em identidade de produção.
- **Histórico e falha relatada:** resolver de imagens antigo ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`; causa/solução precisam de reprodução e testes. Tursogo original permanece referência histórica/read-only para migração quando autorizado, não datastore único do futuro.

## Identidades e segurança MCP

Sessão MTProto Telegram = credencial operacional sensível do coletor. `subscription_id` = escopo configurado de aquisição conforme contrato de Evidence, não canal nem usuário automaticamente. Eventual identidade de consumidor Limiar e autorização MCP = outra responsabilidade, ainda **não comprovada como implementada**. Investigar possibilidade real de reaproveitamento de autenticação do Limiar; nunca passar arquivo de sessão, OTP ou senha MTProto ao ChatGPT, nem conceder acesso global só por conectar MCP. Definir scopes de leitura, canais/dados autorizados, revogação e tratamento de mensagens como dados não confiáveis. Não há MCP instalado/conectado nesta entrega; compatibilidade concreta de protocolo e método de integração deverão ser verificados no início da respectiva fatia.

## Requisitos e aceites por momento

**Agora (preparação):** branch somente documental, escopo recuperável, protocolo/guia local, arquitetura proposta, tracker e checkpoint verificáveis; nada de código, criação de ambiente runtime, acesso real Telegram, merge ou deploy.

**Próximo chat (L3-001 investigação):** conferir HEAD e fontes, propor isolamento (branch + diretório ou opção justificada) com trade-offs e ponto de entrada mínimo; identificar owners e contratos para autenticação MTProto e credenciais; revisar ADR 023 contra alternativas e plataformas, investigar a identidade de consumidor apenas se necessária; definir critérios/testes do primeiro slice sem implementá-lo automaticamente. Questões não respondidas devem aparecer como abertas, não como falhas atribuídas ao usuário.

**Primeira implementação futura, mediante autorização:** projeto inicial executável isolado + autenticação/sessão reutilizável após restart; segredo protegido, falha explícita, sem logs sensíveis, testes sob revisão exata. A integração MCP só pode consumir capacidades reais e permissões definidas; primeira consulta ponta a ponta será mensagem autorizada coletada/persistida e encontrada pelo frontend/API e/ou ChatGPT conforme ordem de integração e disponibilidade.

**Operação futura:** comparação funcional e benchmark antes/depois; importação apenas contra cópia histórica descartável autorizada; estratégia de rollback e corte; jamais declarar legado substituído por build verde ou adicionar data de Black Friday como prazo garantido. Objetivo do usuário: Limiar confiável para Black Friday 2026; escopo e aceites temporais detalhados seguem abertos.

## Exclusões desta decisão de escopo

Não aprova topologia de processos, diretórios, mecanismo concreto de storage da sessão, login de consumidores, OAuth/integração externa, fornecedores MCP, schema de propostas, migração, exclusão do legado, metas numéricas, novos custos, merge ou deploy. Não assumir acesso irrestrito à Internet, credenciais, Project ou agendamentos. Decisões técnicas relevantes continuam sob AGENTS e ADRs.