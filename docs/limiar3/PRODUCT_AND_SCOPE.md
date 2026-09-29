# Limiar 3.0 — visão, escopo e critérios iniciais

**Natureza:** requisitos expressos pelo mantenedor nos diálogos de 2026-09-22 e neste pedido; não implica implementação/aceitação automática de mecanismos técnicos. Esta é a fonte de escopo **da frente Limiar 3.0** nesta branch. `legacy/limiar2/docs/REBASELINE_SCOPE.md` registra escopo anterior (reconstrução integral da fundação de ingestão) e permanece preservado como história. Não confundir versões ou declarar que foi atualizado na `main`.

## Produto e problema

Limiar é um agrupador de promoções: monitora fontes como canais do Telegram, coleta mensagens e imagens, preserva Evidence/proveniência, processa, extrai e agrupa informações comerciais, **apresenta dados limpos**, disponibiliza consultas via API e apresenta o resultado em um **frontend de agrupador de promoções**. O Limiar 1 foi interrompido pelo acoplamento excessivo e problemas funcionais, incluindo imagens não coletadas corretamente segundo relato do mantenedor. Não se demonstrou ainda a causa técnica dessas falhas neste plano.

**Direção do Limiar 3.0:** reconstruir **100% da implementação do produto**, não apenas a ingestão nem uma sucessão de remendos no código legado. Preservar comportamentos e dados necessários, ADRs Accepted, experimentos e testes comprovados, sem traduzir cada componente antigo literalmente ou transplantar seus acoplamentos. O legado é referência e fonte de aprendizado, não arquitetura imutável. O novo código convive de forma isolada até cumprir gates de migração e substituição; não alterar dados históricos in-place.

**MCP é parte do produto desde a fundação** e possui duas funções distintas. Primeiro, um **MCP Telegram realtime** consulta o Telegram diretamente, sob demanda e com autorização, sem depender de a mensagem já existir no storage do Limiar; ele serve tanto para investigar o corpus quanto como superfície cotidiana para o ChatGPT consultar os canais cadastrados. O MCP deve fornecer ao ChatGPT histórico, mensagens, metadata/informações dos canais e mensagens e uma representação semanticamente fiel do Telegram, sem entregar credenciais/internals MTProto. **Busca, filtragem semântica, comparação e síntese pertencem ao ChatGPT:** por exemplo, diante de "procure Samsung e compare os menores preços", o ChatGPT usa o MCP para ler/paginar o corpus autorizado e realiza a análise; isso não exige uma tool MCP de busca comercial. Consultas recorrentes como "ficar de olho em promoções de celular" podem ser orquestradas pelo consumidor quando houver automação/polling disponível. Depois, o mesmo MCP pode ganhar **Limiar data tools** sobre as capacidades processadas do produto. O MCP não substitui o collector, não torna uma consulta realtime em Evidence durável automaticamente e sua indisponibilidade não bloqueia coleta/processing. Ele deve ser desacoplável por construção.

## Fluxo conceitual, não topologia definitiva

`Session Credential → Telegram/MTProto capability` forma o boundary de integração com Telegram. A partir dele existem dois ramos: (a) `MCP Telegram realtime → consulta sob demanda ao Telegram`; (b) `collector → Source Admission → Evidence → recovery/backfill → processing → dados limpos → API → frontend`.

O MCP realtime entra **antes da modelagem comercial** para exploração do domínio. Mais tarde, `MCP Limiar data → Query Service` consulta mensagens admitidas e ofertas estruturadas. As duas famílias de tools podem coexistir no mesmo servidor MCP, mas não compartilham authority por conveniência.

**Não existe 'segundo aplicativo Limiar'**; esse termo foi introduzido indevidamente pelo assistente e corrigido pelo mantenedor. As superfícies desejadas são API/frontend próprios e integração ChatGPT/MCP. Não assumir app mobile, plataforma SaaS multiusuário ou sistema de contas completo como requisito implícito.

## Objetivos obrigatórios de engenharia

1. Responsabilidade e lifecycle com owner inequívoco; alta coesão, baixo acoplamento, fronteiras entre domínio/aplicação/transporte/persistência/integrações/apresentação, composição explícita e interfaces apenas quando há motivo real. Não criar dezenas de classes e pastas para parecer sofisticado.
2. Performance, rapidez e eficiência desde a arquitetura: evitar trabalho redundante, cópias e consultas evitáveis; limites de concorrência e I/O; medir baseline e ganhos comparáveis antes de declarar melhoria. Não inventar SLO, volume, taxa de perda ou prazo de entrega garantido.
3. Longo prazo: código Go idiomático, legível, nomes expressivos, modularidade por ownership/motivo de mudança, contratos estreitos, pouca complexidade gratuita, extensão guiada por requisitos reais.
4. Resiliência: integridade, durabilidade, restart/crash, duplicação, idempotência quando necessária, tratamento de falhas parciais, timeout/cancelamento/backpressure/retry, observabilidade, segurança de segredos e isolamento de dados.
5. Testabilidade: testes unitários/contrato/integração e end-to-end nas fatias pertinentes; invariantes, imagens e cenários de falha; critérios de aceite e evidência da revisão exata para afirmar conclusão.
6. Execução incremental **de baixo para cima**: runtime + sessão; boundary Telegram/MTProto desacoplável; MCP realtime cedo para explorar mensagens reais; em paralelo collector/Evidence/recovery/backfill/mídia; somente depois congelar Findings/modelo de promoções com base no corpus observado; então Query Service, MCP de dados, API e frontend. Não transformar desacoplamento em microserviços sem necessidade.
7. Evitar investigação infinita: investigar só incerteza impeditiva; assim que Implementation Gate e autorização da etapa passarem, implementar fatia funcional completa e validar. A preparação documental de hoje não é autorização para código.

## Reaproveitamento classificado por autoridade

- **Accepted:** ADRs 016–020 para Source Evidence, recovery, Source Admission, SQLite local side-by-side, schema de Evidence; ver registry `legacy/limiar2/docs/adr/README.md` para outras transições. ADR 004 preserva princípio de sessão MTProto durável, não escolha perpétua de Tursogo.
- **Implementado em parte:** `internal/storage/sqlite` com Evidence, guards/schema reabertura; PR #211 integrada; reutilização depende compatibilidade do novo wiring e revalidação, não de cópia cega.
- **Evidência experimental:** EXP-LIMIAR-011–017 durabilidade/portabilidade no escopo testado, macOS inconclusivo; EXP-LIMIAR-018 rejeita `gotd.FileStorage` as-is; EXP-LIMIAR-019 Supported para arquivo hardened Unix/intra-processo, PR #202 integrada, apenas `_test.go`, sem mecanismo produtivo ou suporte Windows demonstrado.
- **Proposto, não decidido:** ADR 021 schema SourceSyncState; ADR 022 BackfillProgress; ADR 023 armazenamento hardened da sessão; ADR 024 identidade `subscription_id`. Não colocar proposta em produção sem aceite formal; não converter fixture `telegram:test` em identidade de produção.
- **Histórico e falha relatada:** resolver de imagens antigo ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`; causa/solução precisam de reprodução e testes. Tursogo original permanece referência histórica/read-only para migração quando autorizado, não datastore único do futuro.

## Identidades, MTProto e segurança MCP

Sessão MTProto Telegram = credencial operacional sensível pertencente ao boundary Telegram/MTProto, não ao collector nem ao MCP. `subscription_id` = escopo configurado de aquisição conforme contrato de Evidence. Eventual identidade/autorização do consumidor MCP é outra responsibility. O adapter MCP recebe capabilities; não recebe bytes da sessão, OTP ou senha como dado de aplicação.

A integração gotd/MTProto também deve ser desacoplável do core: detalhes como `gotd.Client`, `tg.*`, `updates.Manager`, sessão e access hashes ficam contidos no adapter Telegram tanto quanto praticável. Desacoplável não implica processo separado; single-process permanece válido até existir Evidence para outra topologia.

Definir scopes de leitura, canais/dados autorizados, revogação e tratamento de mensagens como dados não confiáveis. "Cru" significa dados/metadata Telegram fielmente representados em DTO seguro, não exposição de `tg.*`, access hash ou sessão. O MCP fornece acesso read-only aos targets cadastrados; **o ChatGPT é quem busca, filtra, cruza e compara** sobre o corpus recuperado. Uma consulta MCP realtime não é automaticamente Source Evidence. Polling/automação do consumidor pode repetir consultas, mas watch state server-side não é requisito implícito. Não há MCP instalado/conectado nesta entrega.

## Requisitos e aceites por momento

**Agora (preparação):** branch somente documental, escopo recuperável, protocolo/guia local, arquitetura proposta, tracker e checkpoint verificáveis; nada de código, criação de ambiente runtime, acesso real Telegram, merge ou deploy.

**Próximo chat (L3-001 investigação):** conferir HEAD e fontes, propor isolamento (branch + diretório ou opção justificada) com trade-offs e ponto de entrada mínimo; identificar owners e contratos para autenticação MTProto e credenciais; revisar ADR 023 contra alternativas e plataformas, investigar a identidade de consumidor apenas se necessária; definir critérios/testes do primeiro slice sem implementá-lo automaticamente. Questões não respondidas devem aparecer como abertas, não como falhas atribuídas ao usuário.

**Primeiras implementações futuras, mediante autorização:** projeto inicial + sessão reutilizável; depois boundary Telegram/MTProto estreito; em seguida uma primeira tool MCP realtime read-only pode consultar Telegram diretamente antes de existir modelo comercial. O collector/Evidence é outro ramo e mantém seus próprios gates de durabilidade. Dados de promoções só devem ser congelados depois que a exploração do corpus produzir Findings suficientes. Posteriormente o MCP ganha tools sobre a Query Service processada.

**Operação futura:** comparação funcional e benchmark antes/depois; importação apenas contra cópia histórica descartável autorizada; estratégia de rollback e corte; jamais declarar legado substituído por build verde ou adicionar data de Black Friday como prazo garantido. Objetivo do usuário: Limiar confiável para Black Friday 2026; escopo e aceites temporais detalhados seguem abertos.

## Exclusões desta decisão de escopo

Não aprova topologia de processos, diretórios, mecanismo concreto de storage da sessão, login de consumidores, OAuth/integração externa, fornecedores MCP, schema de propostas, migração, exclusão do legado, metas numéricas, novos custos, merge ou deploy. Não assumir acesso irrestrito à Internet, credenciais, Project ou agendamentos. Decisões técnicas relevantes continuam sob AGENTS e ADRs.