# Guia de engenharia — Limiar 3.0

**Natureza:** orientação operacional **derivada**, não cópia integral nem substituto do Engineering DNA original (2.171 linhas). O arquivo integral está preservado **byte a byte na branch**, compactado/Base64 em [`ENGINEERING_DNA_ORIGINAL.md.gz.b64`](ENGINEERING_DNA_ORIGINAL.md.gz.b64). Git blob remoto `ceafec243be87ae28ee2d9658e3912831fc55368` coincide com o Git blob calculado da fonte original compactada. Após descompactar, o SHA-256 esperado é `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`. Consultar a entrada [`ENGINEERING_DNA.md`](../../ENGINEERING_DNA.md) para instruções; a transcrição Markdown integral ainda não está descompactada na raiz. A Constituição de `AGENTS.md`, requisitos explícitos e ADRs Accepted prevalecem conforme as autoridades próprias.

## Direção do trabalho

- Observe antes de assumir: confirme branch, HEAD, PRs, permissões, contratos, testes e comportamento; conectores remotos não demonstram worktree local. Distinga Evidence, inferência, hipótese, Proposed e Accepted.
- Uma responsabilidade, um owner definido para criar, validar, mutar, observar e encerrar lifecycle. Separe credencial Telegram, peer cache, configuração de aquisição, Evidence, estado de sync, backfill, interpretação comercial, projeções, acesso MCP, API e apresentação quando tiverem motivos/lifecycles diferentes.
- Fronteiras reais, coesão alta e baixo acoplamento. Transporte != domínio, storage != domínio, configuração != runtime, sessão != perfil, identidade != autorização, dados externos != regras de UI. Composição explícita no entrypoint, dependências para contratos internos quando justificadas.
- **Sem arquitetura ornamental:** não introduzir interfaces por convenção, DI frameworks, registries, abstrações de múltiplas fontes, microserviços ou dezenas de pastas sem demanda real. Co-localizar quando owner e motivo de mudança forem os mesmos. Evitar `utils` genérico, estado global mutável e dependências circulares.
- Código Go idiomático, nomes expressivos com semântica de exportação correta; não dividir por tamanho de arquivo nem fazer renomeações universais.
- Legado é referência de comportamento, regressão, dados históricos e migração, não estrutura-alvo. Isolar compatibilidade e remover apenas depois de prova e autorização. Reusar SQLite/Evidence quando contratos e garantias seguirem válidos; não reescrever por estética.
- **Current Stable First:** para projeto novo, linguagem/toolchain/dependência stable atual é o default de avaliação. Versão anterior exige motivo técnico concreto, documentado e demonstrável. Pinning continua obrigatório; features novas entram seletivamente quando ajudam o escopo real. Fonte detalhada: [`DEPENDENCY_TOOLCHAIN_POLICY.md`](DEPENDENCY_TOOLCHAIN_POLICY.md).

## Correção, segurança e runtime

- Evidence admitida não desaparece silenciosamente. Progresso nunca declara durabilidade inexistente. Falha parcial, reinício, replay, concorrência e backpressure têm semântica explícita, com fail-closed para confiança/credenciais e blast radius mínimo.
- Sessão MTProto é segredo/authority distinto de peer cache e de eventual autorização do usuário para MCP. Nunca registrar bytes, OTP, senha ou arquivo de sessão em logs, documentação, fixtures ou chat. Erro de leitura, permissão, ambiguidade e corrupção não podem virar sucesso silencioso.
- I/O com cancelamento, timeout, retries idempotentes quando aplicáveis, limites de fila e concorrência, lifecycle explícito para sessão/conexões/workers/downloader/MCP. Falha do MCP não pode paralisar admissão, Evidence, processamento nem API/frontend.
- MCP é extensão de consulta do Limiar: acesso mínimo e revogável a mensagens/ofertas autorizadas, limites por identidade, auditoria proporcional e mensagens Telegram como **dados não confiáveis**, não instruções a agentes. Nenhum MCP real ou autorização foi instalado nesta documentação.

## Qualidade mensurável

- Identificar hot paths e workload; baseline de throughput, latência p50/p95 onde relevante, memória, CPU, I/O, backlog e tempo de recuperação. Medir antes/depois em cenários comparáveis. Não atribuir ganhos nem inventar metas numéricas.
- Testes de contrato e regressão protegem comportamento/invariantes, não só build. Para defeitos: reproduzir, identificar boundary/owner, teste que falha, menor correção, executar gates e conferir impacto. Imagens têm relato de defeito, sem causa reproduzida.
- Exercitar duplicidade, crash/restart, rede indisponível, entrada malformada, configuração ausente, credencial inválida, concorrência, encerramento e backpressure conforme a fatia.
- Implementação, validação, revisão, integração e deploy são estados separados. Entrega distingue implementado, validado na revisão/ambiente exatos, não validado, desconhecido e próximo passo. CI antigo não prova novo HEAD.

## Ritmo e documentação

- Reconstruir projeto inteiro em fatias observáveis, **de baixo para cima**: fundação/configuração/autenticação MTProto → admissão/mensagens/Evidence/recovery → mídia → processamento/dados limpos → superfícies MCP/API → frontend. MCP é considerado desde o início e pode ter fatias paralelas assim que existir capability real. Ordem depende de dependências observadas.
- L3-001/A/B/C concluíram a investigação ampla da fundação. A auditoria upstream-only atingiu sua stop condition; daqui em diante, só investigar dúvida material concreta. Resolver decisões bloqueantes, executar experimentos estreitos e aplicar o Implementation Gate quando houver autorização.
- Agent Development Protocol v2.0 `docs/DOCUMENTATION_AND_CONTINUITY.md`: §5 para projeto novo, RECOVER/RECONCILE, ID de tarefa, checkpoint, Implementation Gate e HANDOFF. Evitar loop interminável de ADRs/experimentos; implementar assim que informação suficiente **e autorização** existirem.
- Craftsmanship: solução menor **completa** e comprovada, simples, correta, coesa, segura, legível, eficiente; qualidade não é quantidade de arquivos/interfaces/docs.

**Limite residual:** o original integral está no GitHub **compactado com identidade de blob verificada**, não como Markdown integral de leitura imediata. Descompactar e verificar SHA no ambiente do novo chat antes de afirmar que a fonte completa foi efetivamente consumida. Não inferir sincronização com Project, Codex ou Tarefas Agendadas.