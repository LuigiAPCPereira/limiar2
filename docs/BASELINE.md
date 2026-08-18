# BASELINE — Rebaseline 2026 do Limiar

Authority: Derived
Status: Active Snapshot

Este documento resume o estado arquitetural atual do Limiar de forma legível.
Ele é derivado de `AGENTS.md`, ADRs aceitos e do registry de transição em
`docs/adr/README.md`.

Este arquivo **não cria decisões arquiteturais novas**. Quando houver conflito, a ordem
de autoridade definida em `AGENTS.md` prevalece.

---

## 1. Objetivo do produto

O Limiar é uma plataforma pessoal de agregação e inteligência de promoções.

O sistema deve ser capaz de:

- capturar ofertas de fontes como Telegram preservando a evidência recebida;
- transformar conteúdo semi-estruturado em informação comercial estruturada;
- preservar incerteza e proveniência em vez de fabricar certeza;
- relacionar ofertas equivalentes sem destruir observações individuais;
- disponibilizar resultados por API para experiências de consumo como feed,
  pesquisa, filtros e comparações;
- permitir reprocessamento quando regras, modelos ou interpretações evoluírem.

Telegram é a fonte concreta atual. A baseline não exige um framework genérico de
fontes antes de existir uma segunda necessidade comprovada.

---

## 2. Invariantes vigentes

Os invariantes constitucionais C-01 a C-11 de `AGENTS.md` são autoridade.
Em termos operacionais, a baseline depende especialmente destas propriedades:

1. Evidence admitida não desaparece silenciosamente;
2. estado derivado é reconstruível;
3. progresso não pode certificar durabilidade inexistente;
4. Evidence, Finding, Inference e Decision permanecem separados;
5. processamento determinístico e probabilístico têm contratos distintos;
6. `UNKNOWN` é legítimo quando a evidência é insuficiente;
7. identidade de domínio não é definida por conveniência de feed ou deduplicação;
8. topologia de execução não define autoridade;
9. complexidade precisa de evidência proporcional;
10. agentes propõem e o mantenedor decide;
11. implementação existente não cria autoridade por existência.

---

## 3. Forma conceitual da rebaseline

A direção conceitual produzida pela arqueologia é:

```text
Fonte
  ↓
Evidence
  ↓
Source Projection
  ↓
Deterministic Findings
  ↓
Promotion Interpretation
  ↓
Product / Listing / Offer
  ↓
Relations
  ↓
Feed Projection
```

Enriquecimento probabilístico, inclusive IA, é opcional e lateral ao núcleo
determinístico. Essa forma é uma síntese da rebaseline; os contratos concretos de cada
camada só se tornam autoridade quando cobertos por ADR Accepted.

---

## 4. Estado de decisões e candidatos

### Autoridade já vigente

- a Constituição em `AGENTS.md`;
- o lifecycle de ADR e a transição dos ADRs legados em `docs/adr/README.md`;
- **ADR 016 — Source Evidence e sincronização Telegram**, que estabelece:
  - Evidence de fonte append-only/versionada;
  - separação entre Evidence e `SourceMessageKey`;
  - sync live baseado no estado nativo do Telegram, não em `LastMessageID`;
  - lifecycle de backfill separado do sync live;
  - durabilidade antes de avanço certificado de sync state;
  - replay permitido sem promessa de exactly-once end-to-end;
  - history snapshot sem fabricação de eventos;
  - gotd mantido como integração Telegram, sem ainda decidir a mecânica concreta de
    recovery/barrier.

Com o ADR 016 aceito, o ADR 005 fica superseded no escopo do contrato raw por mensagem e
o ADR 006 fica retired/superseded no escopo de `LastMessageID` como autoridade de sync
live. O princípio de gotd do ADR 002 é mantido com boundary de sincronização reescrito.

### Candidatos fortes, ainda não autoritativos

Os itens abaixo são resultados de pesquisa, experimentos e propostas da rebaseline.
**Eles não devem ser tratados como decisões apenas por aparecerem aqui.**

- boundary de recovery Telegram Candidate v3, validado contra gotd/td v0.161.0 por 12
  contract tests + race detector: `GuardedRecoveryAPI + updates.Manager +
  DurableEvidenceHandler + GuardedStateStorage + DurabilityBarrier + Supervisor`;
  `PROPOSAL-ING-002` e ADR 017 permanecem não autoritativos enquanto o ADR 017 estiver
  `Proposed`;
- migração side-by-side do banco legado, preservando o antigo como artefato imutável;
- Processing Generations para derivados reconstruíveis e reprocessamento seguro;
- Source Message Projection separada da Evidence;
- Commercial Findings tipados e com proveniência;
- Promotion Interpretation multifacetada em vez de `MessageType` exclusivo;
- separação entre Product, Merchant Listing, Offer Observation e Feed Projection;
- relações explícitas em vez de `IsDuplicate` como propriedade permanente;
- IA opcional, auditável, cacheável e incapaz de bloquear o núcleo determinístico.

Cada item estrutural deve ganhar ou referenciar ADR antes de orientar produção.

---

## 5. Legado

O banco, schema e código atuais são evidência histórica e implementação existente.
Eles não são automaticamente a arquitetura desejada.

Durante a rebaseline:

- dados legados não devem ser descartados silenciosamente;
- testes e benchmarks úteis devem ser preservados como regressão/evidência;
- migrations antigas continuam relevantes para importar e compreender o banco legado;
- componentes classificados para substituição só devem ser removidos depois que sua
  função, evidência útil e caminho de migração estiverem cobertos.

O registry de `docs/adr/README.md` governa a interpretação dos ADRs 001–015 em conjunto
com ADRs novos já aceitos.

---

## 6. Stack atual observada

A implementação atual usa, entre outras dependências:

- Go 1.26.2 no `go.mod`;
- `github.com/gotd/td` v0.161.0 para Telegram/MTProto;
- `turso.tech/database/tursogo` v0.7.2 para storage local;
- Cobra/Viper para CLI/config;
- `net/http` para o dashboard atual.

Essas versões descrevem o repositório atual. Não são congeladas pela baseline.
Mudanças estruturais seguem `AGENTS.md` e ADRs; updates compatíveis devem ser avaliados
com documentação primária e testes proporcionais ao risco.

---

## 7. Questões abertas prioritárias

1. decisão explícita sobre o boundary de recovery Telegram Candidate v3 (ADR 017);
2. escolha/validação da engine SQLite local e PRAGMAs de durabilidade;
3. contrato físico do payload de Evidence;
4. estratégia final de sessão/peer state;
5. boundary de mídia entre evidência de fonte e cache/apresentação;
6. materialização dos contratos de Processing Generations e Source Projection;
7. migração do banco legado;
8. escolha de provider/modelo de IA somente quando houver corpus e credenciais para
   bake-off real.

---

## 8. Regra de leitura deste documento

Termos como **candidate**, **proposed**, **open** ou equivalentes permanecem
não-autoritativos.

A presença de um desenho neste documento não substitui:

```text
Proposal
  ↓
ADR Proposed
  ↓
aceitação explícita do mantenedor
  ↓
ADR Accepted
  ↓
implementação de produção
```

A baseline deve ser atualizada quando decisões aceitas alterarem o estado descrito aqui.
