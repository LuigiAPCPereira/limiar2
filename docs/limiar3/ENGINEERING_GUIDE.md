# Guia de engenharia — Limiar 3.0

**Natureza:** orientação operacional **derivada**, não cópia integral nem substituto do `ENGINEERING_DNA.md` original do ChatGPT Project (2.171 linhas, SHA-256 da cópia anexada `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`). A cópia integral ainda precisa ser colocada e confrontada na branch para que outros agentes possam utilizá-la sem depender do Project. Não afirmar equivalência de versões sem comparar bytes/hash. A Constituição de `AGENTS.md`, requisitos explícitos do mantenedor e ADRs Accepted prevalecem conforme suas autoridades próprias.

## Direção do trabalho

- Observe antes de assumir: confirme branch, HEAD, PRs, permissões, contratos, testes e comportamento relevante; conectores remotos não demonstram estado de worktree local. Distinguir Evidence, inferência, hipótese, Proposed e Accepted.
- Uma responsabilidade, um owner definido para criação, validação, mutação, observação e fim de lifecycle. Separe credencial Telegram, peer cache, configuração de aquisição, Evidence, estado de sync, backfill, interpretação comercial, projeções, acesso MCP, API e apresentação quando tiverem motivos/lifecycles diferentes.
- Fronteiras reais, coesão alta e acoplamento baixo. Transporte != domínio, storage != domínio, configuração != runtime, sessão != perfil, identidade != autorização, dados externos != regras de UI. Composição explícita no entrypoint e dependências para contratos internos quando justificadas.
- **Sem arquitetura ornamental:** nenhuma interface apenas por convenção, frameworks de DI, plugins, registries, abstrações de várias fontes, microserviços, dezenas de pastas ou camadas sem consumidor/problema demonstrado. Co-localizar quando owner e motivo de mudança forem os mesmos. Evitar `utils` genérico, estado global mutável e dependência circular.
- Código Go idiomático, nomes expressivos que respeitam semântica de exportação, funções pequenas onde isso melhora responsabilidade; não dividir só por tamanho de arquivo nem fazer renomeações universais.
- Legacy é referência de comportamento, regressões, dados históricos e migração; não estrutura-alvo. Isolar compatibilidade e remover apenas depois de prova e autorização. Reusar código SQLite/Evidence se seus contratos e garantias se mantiverem no novo wiring; não reescrever por estética.

## Correção, segurança e runtime

- Evidence admitida nunca desaparece silenciosamente. Progresso nunca declara durabilidade inexistente. Falha parcial, reinício, replay, concorrência e backpressure têm semântica explícita, com fail-closed para confiança/credenciais e blast radius mínimo.
- Sessão MTProto é segredo e authority distintos de peer cache e de eventual autorização do usuário para MCP. Nunca registrar bytes, OTP, senha ou arquivo de sessão em logs, documentação, fixtures ou chat. Falta de permissão, erro de leitura, ambiguidade de identity/configuração e corrupção não podem virar sucesso silencioso.
- I/O com cancelamento, timeouts, retries justificáveis/idempotentes, limites de fila e concorrência; lifecycle explícito para sessão, conexões, workers, downloader e MCP. Falha do MCP não pode paralisar admissão, Evidence, processamento nem API/frontend.
- MCP é extensão de consulta do Limiar: acesso mínimo e revogável apenas a mensagens/ofertas autorizadas, limites por identidade, auditoria proporcional e tratamento de mensagens Telegram como **dados não confiáveis**, não instruções a agentes. Nenhum MCP real ou autorização foi instalado nesta documentação.

## Qualidade mensurável

- Localizar hot paths e identificar workload; estabelecer baseline de throughput, latência p50/p95 onde relevante, memória, CPU, I/O, backlog, tempo de recuperação e cobertura de falhas. Medir antes/depois em cenários comparáveis. Não atribuir ganho nem inventar metas numéricas.
- Testes de contrato e de regressão protegem comportamento e invariantes, não apenas construção do pacote. Para defeitos: reproduzir, identificar boundary/owner, teste que falha, menor correção, executar gates e verificar impacto. Imagens têm relato de defeito, não causa reproduzida.
- Exercitar cenários de duplicidade, crash/restart, perda de rede, entrada malformada, configuração ausente, credencial inválida, concorrência, encerramento, dados desconhecidos e backpressure conforme a fatia.
- Implementação, validação, revisão, integração e deploy são estados separados. Ao entregar, declarar implementado, validado (com commit/ambiente/comando), não validado, desconhecido e próximo passo. CI histórico não prova HEAD novo.

## Ritmo e documentação

- Projeto inteiro será reconstruído em fatias observáveis, **de baixo para cima**: fundação/configuração/autenticação MTProto → admissão/mensagens/Evidence/recovery → mídia → processamento/dados limpos → superfícies de consulta MCP/API → frontend; MCP deve ser considerado desde o início e pode ganhar fatias paralelas assim que existir capability real. Ordem exata depende de dependências observadas, não de árvore fixa.
- Primeiro bloco do próximo chat é **investigar** projeto inicial + autenticação/sessão (`L3-001`), não codificar por efeito do handoff. Resolver só decisões que bloqueiam o primeiro slice. Consultar documentação primária de gotd/MCP somente quando materially necessário e verificar versões atuais e versionadas do repositório.
- Usar Agent Development Protocol v2.0 `docs/DOCUMENTATION_AND_CONTINUITY.md`: §5 para projeto novo, RECOVER/RECONCILE proporcional, tracker ID, checkpoint, Implementation Gate e fase de HANDOFF. Não tornar o processo uma fila infinita de ADRs/experimentos; implementar quando informação suficiente E autorização da etapa existirem.
- Engenharia inspirada por craftsmanship: solução menor **completa** e comprovada; simples, correta, coesa, segura, legível, eficiente e sustentada por evidências. Qualidade não é quantidade de arquivos, interfaces ou documentos.

**Lacuna documental explícita:** este guia viabiliza orientação imediata, mas o pedido de colocar o Engineering DNA **integral** na branch só estará concluído quando `ENGINEERING_DNA.md` original for efetivamente criado, reaberto e seu SHA-256 comparado à cópia fonte. Não esconder essa pendência em relatório de conclusão.