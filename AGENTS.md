# AGENTS.md — Constituição de desenvolvimento do Limiar

Authority: Constitutional

Este arquivo define as regras duráveis para agentes de IA e contribuidores do Limiar.
Ele não define sozinho a arquitetura concreta, a stack completa, o schema do banco ou
fornecedores permanentes. Essas decisões pertencem a ADRs aceitos e à documentação
derivada deles.

O objetivo desta constituição é impedir que descoberta, experimento, preferência de
agente, código existente ou documentação histórica sejam confundidos com autoridade.

---

## 1. Idioma

Salvo instrução explícita e autorizada do mantenedor em contrário, o idioma humano padrão
do Limiar é **Português do Brasil (pt-BR)**.

Devem ser produzidos em pt-BR: documentação permanente, ADRs, EvolutionDocs, TASKLIST,
ROADMAP, checkpoints e relatórios; títulos e descrições de issues/commits/PRs escritos
pelo agente; comentários e docstrings; textos de interface/CLI; validações, notificações,
mensagens operacionais e erros destinados ao usuário; logs destinados à leitura humana;
e textos de testes/fixtures que representem conteúdo exibido pelo produto.

Não traduzir contratos técnicos por estética. Identificadores Go/TypeScript, nomes de
bibliotecas/frameworks, APIs, campos de payload/schema externo, comandos, flags e nomes
públicos compatíveis permanecem na grafia exigida pelo código, ecossistema ou contrato.
**Código idiomático não implica texto humano em inglês.**

Texto legado em outro idioma deve ser migrado por fatias verificáveis quando a área for
tocada e a mudança puder ser feita sem alterar semântica ou compatibilidade. Não executar
renomeações massivas de identificadores ou contratos apenas para localizar texto.

---

## 2. Ordem de autoridade

Quando houver conflito, use esta ordem:

1. instrução explícita do mantenedor;
2. este `AGENTS.md`;
3. ADRs com `Status: Accepted`, observada a regra de transição em `docs/adr/README.md`;
4. `docs/BASELINE.md`;
5. specs e documentação derivadas de decisões aceitas;
6. implementação atual e testes, apenas como evidência do comportamento existente;
7. EvolutionDocs: findings, experimentos e propostas;
8. documentação histórica, audits, plans, prompts e artefatos de agentes;
9. suposições do agente.

Uma fonte inferior não pode alterar silenciosamente uma fonte superior.

Código ou teste existente não se torna autoridade arquitetural apenas por existir ou
ter permanecido em produção.

Se duas fontes de autoridade divergirem, registre a inconsistência antes de prosseguir.
Não escolha silenciosamente a versão mais conveniente.

---

## 3. Estados epistemológicos

No Limiar, estes conceitos são diferentes:

- **Evidence** — observação direta;
- **Finding** — conclusão suportada por Evidence;
- **Hypothesis** — explicação ou solução ainda não validada;
- **Experiment** — teste controlado de uma hipótese;
- **Proposal** — mudança recomendada;
- **Decision** — escolha arquitetural aceita.

Em particular:

`Finding != Proposal != Decision`

Resultado de experimento `Supported` também não equivale a `Accepted`.

---

## 4. Autoridade dos agentes

Agentes podem:

- pesquisar e analisar código, dados e documentação;
- registrar Evidence e Findings;
- formular hipóteses;
- executar experimentos isolados;
- criar Proposals;
- redigir ADRs com `Status: Proposed`;
- implementar mudanças autorizadas;
- recomendar aceitação ou rejeição de uma decisão.

Agentes não podem, sem instrução explícita do mantenedor:

- promover um ADR de `Proposed` para `Accepted`;
- transformar hipótese, experimento ou Proposal em arquitetura de produção;
- modificar uma invariante constitucional;
- declarar fornecedor ou dependência como obrigatório permanentemente;
- apagar evidência histórica material durante reestruturação.

Um ADR `Proposed` não autoriza uma mudança arquitetural de produção.

A promoção `Proposed -> Accepted` deve deixar prova durável da aceitação do mantenedor
no próprio ADR, com `Accepted-by`, `Accepted-at` e `Acceptance-reference` apontando para
PR, issue, commit ou outro registro verificável. Uma conversa pode autorizar a ação
corrente, mas a decisão final deve ficar registrada no repositório.

---

## 5. Invariantes constitucionais

### C-01 — Evidence admitida não desaparece silenciosamente

Uma observação que o sistema declarou como admitida não pode ser descartada
silenciosamente por backpressure, concorrência, timeout ou conveniência interna.
Replay e duplicidade explícita são preferíveis à perda silenciosa.

### C-02 — Estado derivado é reconstruível

Informação derivada deve ser reconstruível a partir de Evidence, regras, configuração
e versões relevantes. Estado derivado não deve virar uma segunda fonte irrecuperável de
verdade.

### C-03 — Progresso não pode certificar durabilidade inexistente

Cursor, checkpoint, sync state ou outra posição operacional não pode avançar de forma
que declare seguro algo que ainda não possui a durabilidade exigida pelo contrato.

### C-04 — Evidence, Finding, Inference e Decision permanecem separados

Não promover inferência a fato, Finding a Decision ou dado de apresentação a identidade
de domínio.

### C-05 — Determinístico e probabilístico são epistemicamente diferentes

Resultados determinísticos e probabilísticos devem possuir contratos, proveniência e
lifecycle distinguíveis. Dependência probabilística não ganha autoridade implícita
sobre estado determinístico.

### C-06 — UNKNOWN é um estado legítimo

Ausência de evidência não equivale automaticamente a falso, vazio ou inexistente.
Quando não houver base suficiente, preserve a incerteza.

### C-07 — Identidade não é definida por conveniência de apresentação

Agrupamento de feed, URL afiliada, preço semelhante, hash conveniente ou outro artefato
de UX não pode virar identidade canônica sem evidência adequada.

### C-08 — Topologia não define autoridade

Processo, goroutine, canal ou serviço são mecanismos de execução. Autoridade, ownership,
lifecycle, durabilidade e failure domain devem ser definidos explicitamente.

### C-09 — Complexidade precisa de evidência proporcional

Entre duas soluções que satisfazem os mesmos requisitos, prefira a menor. Novas
abstrações, processos, goroutines, dependências e mecanismos distribuídos precisam
resolver um problema demonstrado.

### C-10 — Agente propõe; mantenedor decide

Mudanças arquiteturais permanentes exigem aceitação explícita do mantenedor.

### C-11 — Implementação não cria autoridade por existência

Código, schema, teste, default ou comportamento já presente no repositório não se torna
Decision apenas porque foi implementado ou permaneceu em produção.

---

## 6. O que conta como mudança arquitetural

A classificação depende do efeito da mudança, não do rótulo usado pelo autor.

Uma mudança provavelmente requer ADR quando altera:

- source of truth;
- modelo de durabilidade;
- sincronização, checkpoint ou cursor;
- modelo de dados durável;
- identidade de domínio;
- lifecycle de processamento;
- semântica de reprocessamento;
- side effects externos;
- boundary de segurança;
- topologia de processos quando semanticamente relevante;
- dependência estrutural;
- default que altere persistência, side effects, segurança, autoridade ou semântica
  observável.

Chamar algo de "refactor", "cleanup", "experiment" ou "config change" não remove seu
impacto arquitetural.

Correções locais, testes, documentação e refatorações sem mudança semântica não exigem
ADR por padrão. Governança deve ser proporcional ao risco.

---

## 7. Lifecycle de ADR

Estados permitidos:

- `Proposed`
- `Accepted`
- `Rejected`
- `Superseded`

Transições:

`Proposed -> Accepted`
`Proposed -> Rejected`
`Accepted -> Superseded`

Somente uma decisão aceita posterior pode substituir a autoridade de um ADR Accepted.

ADRs anteriores à Rebaseline 2026 devem ser interpretados segundo o registry de
transição em `docs/adr/README.md` até sua disposição final.

---

## 8. Regra para produção e experimentos

Antes de implementar uma mudança arquitetural permanente:

1. identifique a decisão que a autoriza;
2. confirme que o ADR correspondente está `Accepted`;
3. implemente dentro do escopo aceito;
4. atualize documentação derivada no mesmo conjunto de mudanças.

Sem ADR Accepted, trabalho arquitetural só pode existir como experimento isolado ou
código claramente não autoritativo.

Experimento sem ADR Accepted não pode alterar o comportamento padrão de produção. Em
particular, não pode:

- mudar schema canônico;
- mudar defaults de produção;
- executar side effect real por padrão;
- alterar API pública de produção;
- alterar source of truth;
- tornar dependência obrigatória;
- ser requisito para o build ou run normal do produto.

---

## 9. EvolutionDocs

Pesquisa e evolução técnica devem usar `docs/evolution/`.

Fluxo recomendado quando necessário:

`Evidence -> Finding -> Hypothesis -> Experiment -> Proposal -> ADR`

Nem toda mudança precisa percorrer todas as etapas. Quanto maior a incerteza,
irreversibilidade ou risco, maior deve ser a evidência antes da decisão.

Consulte `docs/evolution/README.md`.

---

## 10. Dependências externas

Não assumir comportamento de biblioteca, API, protocolo ou fornecedor pela memória do
agente.

Quando o comportamento externo puder alterar uma decisão:

1. verificar a versão realmente usada;
2. consultar documentação primária/oficial atual;
3. consultar changelog ou release notes quando relevante;
4. testar comportamento quando documentação não for suficiente;
5. registrar a Evidence que sustenta a conclusão.

Prefira versões stable. Não atualizar dependência apenas porque existe versão mais nova,
mas também não manter versão antiga apenas porque já está no projeto.

Uma dependência é estrutural quando controla, por exemplo, persistência, protocolo de
fonte, runtime principal, provider obrigatório, framework principal de API,
orquestração ou boundary de segurança. Adição ou troca estrutural exige evidência e,
quando muda arquitetura, ADR.

Patch/minor update que não altera contrato não exige automaticamente novo ADR.

Skills e guias auxiliares são ferramentas de trabalho, não fontes superiores à
documentação primária nem aos ADRs aceitos.

---

## 11. Engenharia Go

Estas são regras de engenharia, não decisões arquiteturais sobre packages concretos.

- operações de I/O ou canceláveis devem respeitar `context.Context`;
- não introduzir estado global mutável escondido;
- não usar `panic` como tratamento normal de erro;
- preservar causa e contexto de erros;
- concorrência deve possuir ownership e lifecycle explícitos;
- não criar goroutine, channel ou worker pool por hábito;
- bug corrigido deve possuir teste de regressão quando praticável;
- código concorrente relevante deve ser exercitado com race detector;
- dinheiro não deve ser representado por `float64`;
- evitar abstrações cuja necessidade ainda não exista.

Use a menor solução que satisfaz corretamente o contrato.

---

## 12. Segurança

- nunca logar segredos;
- credenciais não entram em commits, fixtures ou datasets;
- erros não devem expor tokens, sessões ou chaves;
- persistência de segredo exige necessidade e proteção explícitas;
- código de teste deve respeitar os mesmos boundaries de segredo das integrações reais
  quando aplicável.

---

## 13. Verificação

Toda mudança precisa possuir uma forma objetiva de verificação.

Quando aplicável ao código Go:

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Não declare uma tarefa concluída se a verificação relevante não foi executada ou se uma
limitação conhecida impede a conclusão. Registre a limitação explicitamente.

Teste legado prova comportamento existente; não prova que esse comportamento deve ser
preservado quando uma Decision aceita muda o contrato.

---

## 14. Documentação derivada

`docs/BASELINE.md`, `docs/ARCHITECTURE.md`, README e specs explicam o estado atual, mas
não criam autoridade arquitetural independente.

A direção correta é:

`Accepted ADR -> implementação -> documentação derivada`

Nunca:

`edição de ARCHITECTURE.md -> nova arquitetura por acidente`

Se documentação derivada divergir de uma Decision aceita, a documentação deve ser
corrigida.

---

## 15. Código legado, rebaseline e remoção

Durante uma migração ou rebaseline:

- não assumir que código existente representa arquitetura desejada;
- não apagar conhecimento útil apenas porque a implementação será substituída;
- preservar testes, dados, benchmarks e Evidence que ainda possam validar a substituição;
- substituir autoridade antes de destruir o legado.

Classificações como `DELETE-LATER` não são autorização de deleção. Remoção significativa
só deve ocorrer quando substitutos necessários existirem, referências úteis tiverem sido
migradas e a rastreabilidade histórica material estiver preservada.

---

## 16. Processo mínimo de trabalho

Antes:

1. determine o contrato afetado;
2. consulte a autoridade relevante;
3. identifique incertezas materiais;
4. decida se é implementação, experimento ou proposta;
5. defina como verificar.

Durante:

- faça mudanças com escopo consciente;
- não expanda arquitetura incidentalmente;
- registre descobertas que invalidem premissas.

Depois:

- execute a verificação relevante;
- atualize documentação derivada se necessário;
- informe limitações e decisões pendentes;
- nunca promova silenciosamente um artefato epistemológico.

---

## 17. Regra final

Não confunda confiança com Evidence.

Não confunda código com autoridade.

Não confunda Proposal com Decision.

Não complique antes de demonstrar a necessidade.

Preserve a verdade observável e torne a evolução auditável.

---

## 18. Entrada operacional — Agent Development Protocol v2.0

Para desenvolvimento multissessão, ler primeiro este `AGENTS.md` e a fonte do protocolo [`docs/DOCUMENTATION_AND_CONTINUITY.md`](docs/DOCUMENTATION_AND_CONTINUITY.md), na **mesma ref Git** do trabalho. O protocolo v2.0 governa recuperação, adoção, inventário, checkpoint, validação e handoff, sem mudar a ordem de autoridade da seção 2, aceitar ADRs ou conceder permissões. Em 2026-09-29, a fonte canônica na branch foi atualizada com a política explícita de idioma humano pt-BR; o blob atual do protocolo é `5b1cc7900b212ad21a916d30d68ebaacfa06c3f7` (commit `a87398e10d18aee7b3d91c084ec84538ac2a75c4`). A cópia anterior anexada ao ChatGPT Project, cujo SHA-256 medido era `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, **não deve mais ser presumida idêntica** até nova verificação/sincronização. Não afirmar sincronização automática com Project, Codex ou tarefas agendadas.

### Mapa documental das nove funções

| Função | Fonte neste repositório | Estado na adoção |
| --- | --- | --- |
| Identidade, público e limites | `README.md`, `docs/PRODUCT_BRIEF.md`, `docs/BASELINE.md` | parcial; reconciliar fonte vigente |
| Requisitos e aceites | `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/specs/` e ADRs aplicáveis | parcial; cobertura do escopo ativo a conferir |
| Arquitetura e contratos | `docs/BASELINE.md`, `docs/ARCHITECTURE.md`, ADRs Accepted | parcial; ADR 018/BASELINE reconciliados, demais contratos por conferir |
| Decisões duráveis | `docs/adr/README.md`, ADRs Accepted; proposals não são decisões | verificada quanto ao registro |
| Inventário de tarefas | [`docs/TASKLIST.md`](docs/TASKLIST.md) | inventário de adoção, escopo do produto ainda incompleto |
| Planejamento e marcos | [`docs/ROADMAP.md`](docs/ROADMAP.md), `docs/CONTEXT.md`, `docs/BASELINE.md` | roadmap de adoção verificado; planejamento global a conferir |
| Histórico recuperável | `docs/evolution/`, ADRs, PRs e commits | equivalência completa a conferir |
| Checkpoint/ação | [`docs/PROJECT_STATE.md`](docs/PROJECT_STATE.md) | checkpoint de adoção criado; reconciliar HEAD |
| Instruções/versão | Este arquivo e [`docs/DOCUMENTATION_AND_CONTINUITY.md`](docs/DOCUMENTATION_AND_CONTINUITY.md) | fonte acessível na branch, integridade comprovada; outros ambientes não verificados |

O relatório com evidências e lacunas é [`docs/ADOPTION_REPORT.md`](docs/ADOPTION_REPORT.md). Nenhum desses links confirma leitura de Codex ou agendamentos; verificar disponibilidade real em cada execução. `docs/TASKLIST.md` não substitui um inventário de todo o escopo ativo enquanto os itens de produto estiverem incompletos. Manter as permissões reais e os gates constitucionais desta Constituição.
