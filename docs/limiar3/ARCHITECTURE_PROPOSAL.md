# Limiar 3.0 — proposta arquitetural e plano inicial

**Estado:** PROPOSTA PARA INVESTIGAÇÃO, não desenho final, ADR Accepted, autorização de código ou promessa de arquitetura pronta. Fonte de produto: [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md). Regras: `AGENTS.md`, protocolo v2, `docs/adr/README.md` e Accepted. Escopo expandido em conversa pelo mantenedor: reconstrução integral do agrupador de promoções com MCP. O legado não define estrutura obrigatória.

A herança comprovada da Rebaseline 2026 está consolidada em [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md). A construção bottom-up por authorities, gates e slices está em [`BOTTOM_UP_REBUILD_PLAN.md`](BOTTOM_UP_REBUILD_PLAN.md). Este arquivo permanece como proposta arquitetural de alto nível e não substitui esses registros nem ADRs.

## Alternativas de isolamento — decisão ainda investigável em L3-001

| Alternativa | Benefício | Risco/trade-off | Condição de uso |
| --- | --- | --- | --- |
| Branch isolada apenas | Histórico preservado e CI/PR separados | Reescrever pacotes existentes na própria branch favorece novo entrelaçamento, divergência prolongada | Só se a inspeção provar que a estrutura existente comporta responsabilidades novas sem acoplamento. |
| Diretório novo apenas em `main` | Comparação simultânea simples | Código incompleto pode entrar nos gates/empacotamento da main | Não como isolamento de experimentação sem proteção e escopo claros. |
| **Branch dedicada + implementação nova estruturalmente isolada** | Liberdade para reorganizar e preservar legado, commits/PRs rastreáveis | Conflitos com main e duplicidade prolongada; exigir integrações por fatias e config separada | Hipótese inicial preferida, sujeita à verificação do ambiente e compatibilidade de reutilizar `internal/storage/sqlite`. |
| Outro repositório ou segundo módulo Go | Isolamento maior | Duplica dependências/governança, torna aprendizado e integração mais custosos | Só se requisito comprovado justificar. |

Esta branch `docs/limiar-3-foundation-20260922` é **somente preparação documental**; ser descendente da branch de adoção v2 evita perder entrada/protocolo, mas **não** torna o PR #214 merged nem autoriza a próxima branch de código. Confirmar base real de uma futura branch de implementação e se aplicar cherry-pick/merge documental é seguro/autorizado. Não criar code skeleton prematuro.

## Mapa inicial de ownership — hipóteses verificáveis

| Capacidade | Owner de estado/comportamento | Entrada → saída e limites | Falha que precisa ser prevista |
| --- | --- | --- | --- |
| Configuração/composição | Entrypoint/composition root pequeno, validando config | Parâmetros explícitos → dependências construídas | Config ausente/ambígua falha antes de admissão; nenhum segredo em log. |
| Autenticação e sessão MTProto | Boundary de sessão/credencial, separado de peer cache | Fluxo gotd ↔ credencial persistente | Arquivo ausente, permissivo, corrupto, writers, restart, shutdown; ADR 023 Proposed, mecanismo não escolhido. |
| Peer cache | Adapter Telegram/cache com autoridade operacional própria | Peer info ↔ cache; não fonte de sessão | Cache perdido reconstruível conforme contrato a definir; não mesclar secret e cache por conveniência. |
| Admissão de fonte | Boundary de aquisição configurada | Envelope Telegram + subscription configurada → Evidence admitida durável | Append falha → não forward; identity não inferida do canal; ADR 024 Proposed. |
| Evidence | Storage capability SQLite | Payload opaco byte-preserving → Evidence append-only/proveniência | Não sobrescrever, não perder admitidos; integrity e crash; ADRs 019/020 Accepted. |
| Sync/recovery | Gotd updates.Manager com guards/capabilities | Estado de protocolo ↔ sync; durabilidade Evidence antes de progresso | Restart, bootstrap, difference too long, fail-stop, replay; ADRs 016–018 Accepted; schema 021 Proposed. |
| Backfill | Worker com estado/progresso distinto | Intervalos históricos → Evidence, avanço seguro | Cancelamento, rate limit, retomar; ADR 022 Proposed. |
| Mídia | Downloader/armazenamento de referências e conteúdo conforme contrato futuro | Mensagem/media IDs → imagem verificável e rastreável | Imagem ausente, download parcial, referência expirada, album, retry idempotente; bug legado relatado sem causa identificada. |
| Processamento/agrupamento | Serviços de domínio/aplicação independentes de Telegram | Evidence/projeções → findings, interpretação, produtos/listings/offers/relations | Duplicidade, preço desconhecido, contradição e reprocessamento; `UNKNOWN` legítimo. |
| Consulta | Serviço de aplicação único ou capabilities estreitas conforme demanda real | Mensagens admitidas, ofertas estruturadas → resultados autorizados | Ausência, dados desatualizados, origem não acessível; não atribuir menor preço do mercado sem pesquisa externa. |
| MCP | Adapter/exposição de ferramentas sobre capacidades de consulta | Pedido ChatGPT autenticado/autorizado → resultados/proveniência | Indisponibilidade não bloqueia ingress/processing; mensagens são dados não confiáveis; segredo Telegram nunca sai. |
| API e frontend | API traduz contratos de consulta, frontend apresenta agrupador | Resultado limpo → feed, filtros, comparações e estados UI | Loading/empty/error/partial, privacidade, latência, acessibilidade; não duplicar business rules. |

Esses owners são **proposta de fronteiras**, não nomes obrigatórios de packages, binários, tabelas ou número de processos. Revisar dependências reais e evitar criar uma interface para cada linha. Telegram é a fonte concreta atual; framework genérico de providers só com segunda demanda efetiva. AI probabilística permanece lateral ao núcleo determinístico enquanto contratos não determinarem o contrário.

## Fluxo e dependências propostas

```text
Telegram → sessão/autenticação + Source Admission → Evidence durável (SQLite)
                                                    ├→ sync/recovery/backfill [authorities próprias]
                                                    └→ processamento determinístico/interpretativo
                                                            → ofertas/dados limpos → consultas → API → frontend
                                           ↘ consulta autorizada a mensagens/Evidence
                                            MCP ↔ mesmo serviço de consulta ↔ ofertas processadas
```

O MCP é **capacidade paralela e integrada ao produto**, não etapa obrigatória do pipeline nem coleta independente; pode aparecer cedo oferecendo busca em mensagens já admitidas e crescer para ofertas processadas. A sequência textual do mantenedor 'coleta, MCP, processamento, apresentar dados limpos, API e frontend' é registrada no escopo; este desenho oferece acesso MCP antes e depois de processar sem afetar a durabilidade. Se requisito futuro exigir outro fluxo, revisar explicitamente.

**Autenticação — três conceitos separados:** credencial/sessão MTProto do coletor; `subscription_id` de escopo de aquisição (ADR 020 exige campo, ADR 024 Proposed discute ownership); autenticação/autorização eventual para usuário que consulta o Limiar e concede MCP. O legado documenta `limiar-collector auth` para userbot, não comprova identidade de consumidor. Não assumir que um token MCP pode ser sessão Telegram; mecanismo de OAuth, planos, hospedagem e conexão ChatGPT precisam de pesquisa/documentação atuais na fatia própria.

## Reuso sem carregar legado

- `internal/storage/sqlite` é primeira candidata a capability reaproveitável, sujeita a tests na integração nova; migrar dados nunca in-place, manter Tursogo read-only/histórico enquanto autorizado.
- ADRs 016–020 Accepted orientam invariantes e storage; ADR 017 fail-stop e barriers; ADR 018 Source Admission antes de updates.Manager, ainda com integration gates. Testes/experimentos são evidência do ambiente/revisão onde executados, não implantação Limiar 3.0.
- ADRs 021–024 Proposed exigem escolha formal somente quando bloquearem fatia correspondente. ADR 023 é primeira decisão de sessão a investigar; EXP-019 só prova Unix/intra-processo experimental. PR #202 mergeada em 2026-09-14, sem runtime.
- Preservar inputs legados para caracterizar mensagens e imagens, capturar fixtures sintéticas/anonimizadas com consentimento; não copiar credentials reais ou dados privados para Git/ChatGPT sem autorização. Não se realizou nenhum acesso Telegram nesta etapa.
- Portabilidade SQLite/Linux/ARM64/Windows têm evidências delimitadas; macOS e integração completa permanecem sem comprovação aqui. Importação EXP-007 com banco real descartável pendente. Não prometer equivalência ou performance superior.

## Plano incremental, de baixo para cima (sequência revisável)

**M0 — investigação da fundação, `L3-001`:** recuperar HEAD/código/autoridades; confirmar isolamento mínimo e ownership; comparar sessão atual, ADR 004 transição e ADR 023 Proposed; separar login Telegram de consumidor MCP; definir escopo da primeira fatia, config/segredos/risco/testes e decisões indispensáveis. Produto observável: proposta de primeiro código, testes e gates, não código criado por este handoff.

**M1 — projeto inicial + autenticação/sessão (futura implementação autorizada, `L3-002`):** compor entrypoint isolado sem tocar legacy, autenticar em ambiente autorizado, persistir/reabrir credencial, tratar erros de sessão e shutdown; teste sintético + integração real apropriada. Não fixar armazenamento nem plataforma antes do contrato aceito.

**M2 — admissão, mensagens, Evidence e recovery (`L3-003`):** source configured identity, append-before-forward, replay, fail-stop, estado e backfill separados. Dependências: ADRs Accepted e decisão 024 se requerida; schemas 021/022 quando necessários, cada qual com aceite separado.

**M3 — mídia e processamento (`L3-004`):** reproduzir casos de imagens do legado, coletar mídia confiável, transformar mensagens em dados estruturados e agrupados, preservar unknown/proveniência. Validar fatias separadas onde ownership exigir.

**M4 — MCP e superfícies (`L3-005`):** definir contrato de leitura autorizada cedo, implementar consulta a mensagens assim que Evidence disponível, consultar ofertas quando processamento disponível; API/frontend do agrupador usam capabilities comerciais compartilhadas. MCP pode começar antes de M3 completo quando M2 oferecer resultado verificável. Nenhum segundo app.

**M5 — comparação/substituição (`L3-006`):** testes ponta a ponta, performance antes/depois sob workload semelhante, backup/restore, migração histórica em cópia real descartável e rollback; desligar legado apenas após evidência e autorização.

Nenhuma data, PR, schema, estrutura de diretórios ou arquitetura de contas é presumida. Black Friday 2026 é objetivo do mantenedor, não aceite de cronograma. Priorização por dependência real, não por ordem rígida de commits.

## Primeiro bloco futuro — critérios a confirmar na investigação

Resultado proposto: entrypoint isolado com autenticação MTProto e sessão persistente que possa ser reaberta após restart sem novo login indevido; config validada, erros falham fechados, bytes de sessão não logados, permissões e escrita íntegra conforme plataforma suportada; testes novo/preexistente/permissivo/corrompido/concorrente/cancelamento/restart e smoke com gotd real em ambiente autorizado. **Não considerar esses testes já executados.** Consultar a documentação versionada do gotd e fontes primárias atuais se APIs/semânticas forem determinantes. Testar preservação da sessão antiga apenas se migração for requisito concreto e houver insumo autorizado.

**Implementation Gate:** fatia identificada, dependências essenciais e ownership compreendidos, risco de segredo/platform definido, verificação disponível, ADR necessário aceito e autorização de implementação vigente. Se completo em próxima execução, implementar sem ficar em loop documental; se não, limitar investigação à lacuna impeditiva. Este documento não implementa esse Gate automaticamente.