# Escopo de produto — Rebaseline 2026

**Fonte de autoridade deste recorte:** declaração direta do mantenedor nesta conversa, em 2026-09-22, complementando a explicação de 2026-09-22 sobre os problemas do Limiar 1 e o objetivo da Black Friday de 2026. **Estado:** diretriz de produto autorizada em nível macro; contratos específicos, metas numéricas e aceite integral ainda não definidos. Este documento registra a diretriz do produto, não aceita ADRs `Proposed`, não substitui `AGENTS.md` e não comprova implementação.

## Propósito e motivo

O Limiar 1 foi interrompido porque sua implementação era excessivamente misturada, apresentava problemas de funcionamento, separação insuficiente de responsabilidades e falhas na coleta de imagens. O rebaseline pretende construir uma base organizada, funcional, robusta, sustentável e mais eficiente para viabilizar o Limiar na Black Friday de 2026. A plataforma de agregação e inteligência de promoções e o Telegram como fonte concreta atual estão descritos em [`BASELINE.md`](BASELINE.md) §1; o `PRODUCT_BRIEF.md` e o `CONTEXT.md` preservam a visão e a implementação legadas, não autorizam seus detalhes arquiteturais como novos contratos.

## Escopo explicitamente dirigido pelo mantenedor

- **Reconstruir 100% a fundação de ingestão de baixo para cima**: começar por autenticação, sessão, recebimento de mensagens, coleta e responsabilidades diretamente associadas, em vez de apenas rearranjar o código do Limiar 1 ou remendar o subsistema de imagens isoladamente.
- Estabelecer divisões claras de escopo, ownership, organização e limites entre responsabilidades; priorizar correção, segurança, robustez, manutenção de longo prazo e evolução do produto.
- Buscar desempenho, rapidez e otimização com medição e evidência, não com complexidade especulativa ou alegações sem benchmarks.
- Resolver os problemas funcionais do legado, incluindo coleta de imagens, mediante reprodução, diagnóstico por boundary, testes de regressão e validação da integração quando cada fatia for autorizada.
- Usar o **Engineering DNA** como referência de engenharia, respeitando a Constituição e ADRs aceitos do projeto. A cópia consultada nesta conversa está no ChatGPT Project; `ENGINEERING_DNA.md` na raiz da ref GitHub consultada retornou 404 em 2026-09-22. **Sua disponibilização integral/verificação na mesma ref para outros agentes ainda é pendente; não assumir acesso em Codex/agendamentos.**
- Empregar pesquisa externa atual, quando materialmente necessária a contratos e decisões, verificando documentação primária, versão efetivamente usada e testes; acesso à Internet depende das ferramentas e permissões da execução, não é irrestrito.

## Ordem de execução proposta, não nova decisão arquitetural

1. Confirmar os contratos aceitos e delimitar o primeiro slice de autenticação/sessão, seus proprietários, dados sensíveis, lifecycle e testes; não implementar uma proposta estrutural ainda `Proposed` como produção.
2. Avançar para admissão de mensagens, Evidence durável, sincronização e recuperação, preservando a ordem de durabilidade dos ADRs 016–020.
3. Integrar a coleta e o tratamento de mídia com fronteiras explícitas, reproduzindo e prevenindo a falha de imagens relatada; ADR 011 está `DEFER / REVALIDATE` no registry, e ADR 012 está `RETIRE`.
4. Só então avançar em processamento, projeções, exposição e otimização de caminhos críticos conforme contratos e prioridades efetivamente autorizados; não promover fases legadas, IA, frontend ou discovery automaticamente.

A ordem acima é decomposição inicial a validar contra dependências reais, não autorização para reescrever em massa ou para selecionar unilateralmente um mecanismo de sessão, identidade, schema ou topologia.

## Qualidade verificável e limites ainda abertos

Aplicar os princípios do Engineering DNA consultado: observar antes de assumir; ownership e boundaries estreitos; transporte separado de domínio; estado/lifecycle explícitos; falha segura e sem perda silenciosa; integração externa com timeouts, cancelamento e retries controlados; testes de comportamento e regressão; concorrência limitada; métricas antes/depois para otimização. São critérios de método, **não metas numéricas de produto já aprovadas**.

Ainda requerem definição/evidência por fatia: critérios observáveis de autenticação/sessão, formatos e cobertura de mensagens/mídia, taxas aceitáveis de perda/duplicação, volumes e latências-alvo, suporte de plataformas, testes com dados reais descartáveis e disponibilidade operacional desejada para a Black Friday. Os ADRs 021–024 permanecem `Proposed` até aceitação explícita; `docs/BASELINE.md` e `docs/adr/README.md` delimitam as decisões já aceitas. Não inferir que os PRs históricos #121–#147 fazem parte da fundação ou estão cancelados.

**Integração e entrega:** preservar banco, histórico, testes e comportamento útil do legado para migração/regressão, sem transplantar seu acoplamento como alvo. Cada slice exige Implementation Gate e evidência do ambiente/revisão pertinente. Este escopo não autoriza merge, deploy, alteração de segredos ou descarte de dados. A cobertura total do tracker e o Adoption Gate v2 permanecem em avaliação em [`TASKLIST.md`](TASKLIST.md) e [`ADOPTION_REPORT.md`](ADOPTION_REPORT.md).