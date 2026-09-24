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
  - gotd mantido como integração Telegram;
- **ADR 017 — Boundary durável de recovery Telegram**, que complementa o ADR 016 e
  estabelece os contratos do recovery baseado em gotd:
  - `updates.Manager` continua responsável por ordering/recovery do protocolo;
  - o recovery é envolto por responsabilidades equivalentes a `GuardedRecoveryAPI`,
    `DurableEvidenceHandler`, `GuardedStateStorage`, `DurabilityBarrier` e `Supervisor`;
  - Evidence exigida precede a transição de sync state que ela autoriza;
  - barrier check + state write são linearizáveis e uma barrier fechada é terminal para
    aquela instância;
  - erro de leitura de state não é tratado como ausência/bootstrap;
  - bootstrap, resync/reset explícito e `*DifferenceTooLong` são observáveis antes da
    adoção/substituição do baseline;
  - `Forget=true` não é um reset operacional silencioso;
  - updates stateless não definem autoridade de sync;
  - replay após restart é esperado e não implica exactly-once;
- **ADR 018 — Source Admission antes do `updates.Manager`**, Accepted por referência
  explícita à [issue #212](https://github.com/LuigiAPCPereira/limiar2/issues/212):
  - o envelope live atravessa Source Admission durável antes de `updates.Manager`;
  - recovery com Evidence ou mudança de continuidade é preservado antes da adoção pelo manager;
  - o handler pós-Manager não é a única autoridade da Source Evidence bruta;
  - falha de trabalho derivado reconstruível não bloqueia automaticamente sync state se
    a Evidence exigida já estiver durável;
  - fail-stop e recovery do ADR 017 continuam vigentes e backfill mantém progresso independente;
  - aceitação do contrato **não comprova implementação produtiva nem dispensa seus gates**;
- **ADR 019 — Baseline local SQLite para o novo storage**, que estabelece:
  - `github.com/ncruces/go-sqlite3 v0.35.3` como baseline do novo banco através de `database/sql`;
  - uma conexão lógica inicialmente, com `WAL`, `synchronous=FULL`, foreign keys e busy timeout;
  - ordering físico `Evidence -> SourceSyncState/BackfillProgress`;
  - novo banco side-by-side, preservando o Tursogo legado como fonte histórica/read-only;
  - migrations SQL versionadas como única authority de schema do banco novo;
  - capabilities/repositories estreitos em vez de `*sql.DB` global como authority;
  - payload de Evidence opaco/codec-agnostic no storage;
  - `SourceSyncState` e `BackfillProgress` como authorities distintas mesmo no mesmo arquivo físico;
- **ADR 020 — Schema físico mínimo de Evidence**, que estabelece:
  - UUIDv4 via `crypto/rand` persistido como `BLOB(16)`;
  - payload byte-preserving e `payload_sha256` calculado pelo boundary de persistência;
  - timestamps Unix em milissegundos;
  - Evidence repetida permitida sem deduplicação física por hash/message ID;
  - tabela `STRICT`, capability estreita de append e guards persistentes contra `UPDATE`/`DELETE`;
  - ausência inicial de índices secundários sem workload que os justifique.

O primeiro slice autorizado pelos ADRs 019 e 020 já está materializado em `internal/storage/sqlite`: o novo banco é aberto side-by-side, reivindicado por `application_id`, aplica a baseline operacional, executa migrations versionadas e expõe `EvidenceAppender` sem colocar o storage legado sob migração in-place.

Na abertura, o storage recusa versões de schema mais novas que as migrations disponíveis e verifica a presença, o tipo e a tabela associada dos objetos `evidence`, `evidence_no_update` e `evidence_no_delete` antes de entregar a capability. Objetos ausentes ou incompatíveis causam erro, sem reparo automático. Essa verificação reduz o gate de integridade do schema na reabertura; não audita integralmente o SQL dos objetos, o conteúdo histórico ou alterações por outro processo depois da abertura.

Com o ADR 016 aceito, o ADR 005 fica superseded no escopo do contrato raw por mensagem e
o ADR 006 fica retired/superseded no escopo de `LastMessageID` como autoridade de sync
live. O princípio de gotd do ADR 002 é mantido com boundary de sincronização reescrito
pelos ADRs 016 e 017.

Com o ADR 019 aceito, o ADR 001 deixa de orientar o **novo** storage. Tursogo continua
permitido onde for necessário para compatibilidade, importação e rollback do banco legado,
sem criar authority sobre o novo banco.

### Candidatos fortes, ainda não autoritativos

Os itens abaixo são resultados de pesquisa, experimentos e propostas da rebaseline.
**Eles não devem ser tratados como decisões apenas por aparecerem aqui.**

- **ADR 021 — Schema físico de SourceSyncState** está `Proposed`: common state completo por `user_id`, channel PTS por `(user_id, channel_id)`, ausência distinta de zero/erro e setters parciais restritos a state existente. A proposta não autoriza implementação até aceitação explícita do mantenedor;
- **ADR 022 — Schema físico de BackfillProgress** está `Proposed`: progresso próprio por `subscription_id`, ausência distinta de posição zero, avanço monotônico/transacional, `completed` limitado ao lifecycle histórico e Evidence-before-progress. A proposta não autoriza implementação até aceitação explícita do mantenedor;
- **ADR 023 — Boundary hardened de sessão MTProto em arquivo** está `Proposed`: mantém a sessão separada do SQLite de Evidence e propõe, no escopo Unix/Linux sustentado pela Evidence disponível, publicação por temporário no mesmo diretório, sincronização, replace/rename, proteção privada, coordenação intra-processo por path e falha fechada quando a proteção exigida não puder ser estabelecida. Windows, coordenação multiprocesso, criptografia adicional e backup/restore de segredo continuam fora do contrato até Evidence própria. A proposta não autoriza implementação até aceitação explícita do mantenedor;
- política e tooling concretos de importação side-by-side do banco legado;
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
  função, evidência útil e caminho de migração estiverem cobertos;
- o arquivo Tursogo legado não deve ser migrado in-place para o novo schema;
- o novo banco nasce em arquivo separado e o legado permanece read-only durante importação/rollback.

O registry de `docs/adr/README.md` governa a interpretação dos ADRs 001–015 em conjunto
com ADRs novos já aceitos.

---

## 6. Stack atual observada

A implementação atual usa, entre outras dependências:

- Go 1.26.2 no `go.mod`;
- `github.com/gotd/td` v0.161.0 para Telegram/MTProto;
- `turso.tech/database/tursogo` v0.7.2 para o storage legado atualmente em produção;
- `github.com/ncruces/go-sqlite3 v0.35.4` para o novo storage SQLite side-by-side;
- Cobra/Viper para CLI/config;
- `net/http` para o dashboard atual.

A coexistência temporária entre ncruces/SQLite e Tursogo é deliberada: o primeiro atende o novo storage autorizado pelos ADRs 019/020, enquanto o segundo permanece restrito ao legado durante compatibilidade, importação e rollback.

Essas versões descrevem o repositório atual. Não são congeladas pela baseline.
Mudanças estruturais seguem `AGENTS.md` e ADRs; updates compatíveis devem ser avaliados
com documentação primária e testes proporcionais ao risco.

---

## 7. Questões abertas prioritárias

1. materializar e validar o Source Admission real em torno do `updates.Manager` e recovery gotd conforme ADR 018 Accepted e seus gates, após reconciliar dependências de identidade ainda em aberto; aceitação da Decision não equivale a implementação;
2. aceitação ou revisão do ADR 021 antes de materializar o schema físico de `SourceSyncState`;
3. aceitação ou revisão do ADR 022 antes de materializar o schema físico de `BackfillProgress`;
4. auditoria/importação side-by-side contra cópia real do banco legado (EXP-LIMIAR-007): o harness já cria o destino temporário através de `internal/storage/sqlite.Open`, portanto usa migrations e baseline operacionais atuais; o gate continua aberto porque nenhuma cópia histórica descartável real foi exercitada, e o SQL transacional do reconciliador experimental não autoriza uma capability equivalente em produção;
5. completar a validação operacional do novo SQLite fora de Linux X64: backup/restore já possui Evidence executável, locking/WAL foi suportado em Linux X64 pelo EXP-LIMIAR-012, `CGO_ENABLED=0` foi suportado em Linux X64/Go 1.26.2 pelo EXP-LIMIAR-013, cross-build foi suportado para `linux/arm64`, `windows/amd64` e `darwin/arm64` pelo EXP-LIMIAR-014, a suíte `internal/storage/sqlite` executou verde em runner Linux ARM64 nativo pelo EXP-LIMIAR-015 e em worker Windows AMD64 nativo com Go 1.26.2/`CGO_ENABLED=0` pelo EXP-LIMIAR-017; o EXP-LIMIAR-016 permaneceu `Inconclusive` em macOS por ausência de worker executável, e validação do produto completo nessas plataformas continua sem Evidence equivalente;
6. aceitação ou revisão do ADR 023 antes de implementar o novo boundary de sessão MTProto; suporte Windows permanece dependente de Evidence específica para ACL/replace, e peer state continua uma questão separada a ser delimitada antes de implementação estrutural;
7. boundary de mídia entre evidência de fonte e cache/apresentação;
8. materialização dos contratos de Processing Generations e Source Projection;
9. escolha de provider/modelo de IA somente quando houver corpus e credenciais para bake-off real.

Os gates de implementação dos ADRs 017 e 019 — inclusive crash/restart da integração
real, `ChannelDifferenceTooLong`, edit/delete/update composto, coexistência backfill/live,
race detector, preservação do legado e validação nas plataformas efetivamente suportadas — continuam obrigatórios
antes de colocar o novo ingress/storage em produção. O gate `CGO_ENABLED=0 go test` já possui Evidence verde em Linux X64/Go 1.26.2; `internal/storage/sqlite` possui Evidence de execução nativa verde em Linux ARM64/Noble/Go 1.26.2 no escopo do EXP-LIMIAR-015 e em Windows AMD64 no ambiente observado do Travis, Go 1.26.2 e `CGO_ENABLED=0`, no escopo do EXP-LIMIAR-017. Esses resultados não implicam suporte automático do produto completo, suporte a Windows ARM64/UNC nem Evidence de runtime macOS.

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
