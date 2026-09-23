# Escopo de produto — Rebaseline 2026

**Fonte de autoridade deste recorte:** declarações diretas do mantenedor em 2026-09-22 sobre problemas do Limiar 1, reconstrução da fundação e Black Friday de 2026, complementadas pela orientação de reconstruir o produto e desenvolver o Limiar e a integração com ChatGPT em conjunto. **Estado:** diretriz de produto autorizada em nível macro; contratos específicos, metas numéricas e aceite integral ainda não definidos. Este documento não aceita ADRs `Proposed`, não substitui `AGENTS.md` e não comprova implementação.

## Propósito e motivo

O Limiar 1 foi interrompido porque sua implementação era excessivamente misturada, apresentava problemas de funcionamento, separação insuficiente de responsabilidades e falhas na coleta de imagens. O rebaseline pretende construir uma base organizada, funcional, robusta, sustentável e mais eficiente para viabilizar o Limiar na Black Friday de 2026. A plataforma de agregação e inteligência de promoções e o Telegram como fonte concreta atual estão descritos em [`BASELINE.md`](BASELINE.md) §1; o `PRODUCT_BRIEF.md` e o `CONTEXT.md` preservam a visão e a implementação legadas, não autorizam seus detalhes arquiteturais como novos contratos.

## Escopo explicitamente dirigido pelo mantenedor

- **Reconstruir 100% a fundação de ingestão de baixo para cima**: começar por autenticação, sessão, recebimento de mensagens, coleta e responsabilidades diretamente associadas, em vez de apenas rearranjar o código do Limiar 1 ou remendar o subsistema de imagens isoladamente.
- Avaliar e desenvolver uma nova implementação do Limiar como um todo em fatias verificáveis, preservando decisões, evidências, testes, dados e comportamentos úteis sem herdar acoplamentos legados; a proposta de isolamento por branch e código novo deve respeitar uma revisão de origem verificada, testes e integração incremental.
- **Desenvolver em paralelo a integração ChatGPT (app/MCP) e o Limiar**, como duas interfaces para as mesmas capacidades, sem duplicar coleta, catálogo, regras de negócio ou autoridade de dados. A integração deve permitir consultas pessoais a promoções dos canais autorizados; iniciar com busca/leitura e proveniência, acrescentando catálogo processado, histórico e UI apenas conforme capabilities efetivamente entregues.
- Estabelecer divisões claras de escopo, ownership, organização e limites entre responsabilidades; priorizar correção, segurança, robustez, manutenção de longo prazo e evolução do produto.
- Buscar desempenho, rapidez e otimização com medição e evidência, não com complexidade especulativa ou alegações sem benchmarks.
- Resolver os problemas funcionais do legado, incluindo coleta de imagens, mediante reprodução, diagnóstico por boundary, testes de regressão e validação da integração quando cada fatia for autorizada.
- Usar o **Engineering DNA** como referência de engenharia, respeitando a Constituição e ADRs aceitos do projeto. A cópia consultada nesta conversa está no ChatGPT Project; consulta anterior a `ENGINEERING_DNA.md` na raiz da ref GitHub retornou 404. Sua disponibilização integral/verificação na ref para outros agentes ainda é pendente; não assumir acesso em Codex/agendamentos.
- Empregar pesquisa externa atual, quando materialmente necessária a contratos e decisões, verificando documentação primária, versão efetivamente usada e testes; acesso à Internet depende das ferramentas e permissões da execução, não é irrestrito.

## Identidade, autenticação e acesso do ChatGPT — fronteiras a preservar

A autenticação MTProto do coletor é uma sessão/credencial **do Telegram**, não a identidade do usuário do Limiar nem uma autorização concedida ao ChatGPT. A documentação legada descreve um comando CLI `auth` para o userbot, não comprova um serviço de login/OAuth de usuários do Limiar já existente. **É diretriz aproveitar a identidade/autenticação do Limiar caso exista ou quando for definida, sem assumir que a sessão Telegram a substitui.**

Organização-alvo conceitual, ainda não decisão de implementação: Telegram → aquisição autorizada → Evidence durável → processamento/projeções → capacidades de consulta do Limiar → API autorizada → servidor MCP/app ChatGPT. O app e a interface própria devem consumir a mesma regra de acesso e de consulta; a decisão sobre provedor de identidade, formato de token, endpoints, exposição remota e implantação ainda depende de contrato e validação próprios.

Antes de acessar dados pessoais via MCP: autorização pelo usuário, escopos mínimos de leitura, restrição a canais permitidos, verificação da identidade/permissões no servidor em cada chamada, revogação e auditoria sem segredos, isolamento entre usuários e restrição de conteúdo de Telegram como dado não confiável. Não transmitir arquivos de sessão, códigos de login ou credenciais MTProto para o ChatGPT; não publicar endpoint sem autenticação. A documentação oficial de plugins/MCP do ChatGPT requer OAuth compatível e verificação de tokens no servidor para dados privados; conferir requisitos efetivamente vigentes quando iniciar implementação.

## Ordem de execução proposta, não nova decisão arquitetural

1. Confirmar os contratos aceitos e delimitar o primeiro slice de autenticação/sessão Telegram, seus proprietários, dados sensíveis, lifecycle e testes; não implementar uma proposta estrutural ainda `Proposed` como produção.
2. Em paralelo, delimitar o contrato mínimo de consulta e identidade de usuário para o app: retornar dados de teste ou Evidence autorizada com origem e horário, e testar recusa de acesso não autorizado. Sem presumir que login de usuário já está implementado.
3. Avançar para admissão de mensagens, Evidence durável, sincronização e recuperação, preservando a ordem de durabilidade dos ADRs 016–020; conectar a consulta MCP à mesma capacidade persistida somente quando a integração for validada.
4. Integrar a coleta e o tratamento de mídia com fronteiras explícitas, reproduzindo e prevenindo a falha de imagens relatada; ADR 011 está `DEFER / REVALIDATE` no registry, e ADR 012 está `RETIRE`.
5. Avançar processamento, projeções, exposição e otimização dos caminhos críticos conforme contratos e prioridades autorizados; as capacidades do app evoluem junto com cada slice validado, sem promover fases legadas ou IA automaticamente.

A ordem acima é decomposição inicial a validar contra dependências reais, não autorização para reescrever em massa ou selecionar unilateralmente mecanismo de sessão, identidade, schema, topologia ou provedor de autenticação.

## Qualidade verificável e limites ainda abertos

Aplicar os princípios do Engineering DNA consultado: observar antes de assumir; ownership e boundaries estreitos; transporte separado de domínio; estado/lifecycle explícitos; falha segura e sem perda silenciosa; integração externa com timeouts, cancelamento e retries controlados; testes de comportamento e regressão; concorrência limitada; métricas antes/depois para otimização. São critérios de método, **não metas numéricas de produto já aprovadas**.

Ainda requerem definição/evidência por fatia: critérios observáveis de autenticação/sessão, identidade e autorização de usuários Limiar, escopos do app, formatos e cobertura de mensagens/mídia, taxas aceitáveis de perda/duplicação, volumes e latências-alvo, suporte de plataformas, testes com dados reais descartáveis e disponibilidade operacional desejada para a Black Friday. Os ADRs 021–024 permanecem `Proposed` até aceitação explícita; `docs/BASELINE.md` e `docs/adr/README.md` delimitam decisões já aceitas. Não inferir que os PRs históricos #121–#147 fazem parte da fundação ou estão cancelados.

**Integração e entrega:** preservar banco, histórico, testes e comportamento útil do legado para migração/regressão, sem transplantar seu acoplamento como alvo. Cada slice exige Implementation Gate e evidência do ambiente/revisão pertinente. Este escopo não autoriza merge, deploy, alteração de segredos ou descarte de dados. A cobertura total do tracker e o Adoption Gate v2 permanecem em avaliação em [`TASKLIST.md`](TASKLIST.md) e [`ADOPTION_REPORT.md`](ADOPTION_REPORT.md).