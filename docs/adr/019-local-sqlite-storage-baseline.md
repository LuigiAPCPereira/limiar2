# ADR 019 — Baseline local SQLite para o novo storage

Authority: Decision Record
Status: Proposed

> Enquanto este ADR estiver `Proposed`, ele não altera o storage de produção nem autoriza troca de engine, schema, dependência ou banco legado.

## Contexto

Os ADRs 016 e 017 aceitam que Evidence durável preceda o avanço certificado de `SourceSyncState`; o ADR 016 também separa o progresso de backfill da autoridade de live sync. O EXP-LIMIAR-003 suporta a separação entre `SourceSyncState` e `BackfillProgress`.

O storage legado usa `turso.tech/database/tursogo`, expõe `*sql.DB` ao restante do pacote e depende de `PRAGMA synchronous=NORMAL`. O ADR 001 legado está classificado como `SUPERSEDE` no registry da Rebaseline 2026; portanto, sua escolha de Tursogo como datastore único não é autoridade para o novo storage.

O EXP-LIMIAR-002 comparou:

- `github.com/ncruces/go-sqlite3 v0.35.3`;
- `turso.tech/database/tursogo v0.7.2`.

As duas engines passaram o contrato funcional avaliado com `database/sql`, uma conexão, `WAL`, `synchronous=FULL`, foreign keys e busy timeout. Também passaram runtime sem CGO, race detector em gate separado, BLOB/reopen, rollback, `integrity_check`, backup via `VACUUM INTO` e a ordem Evidence-before-state.

O F-STO-002 acrescentou uma matriz física de 48 encerramentos por `SIGKILL` entre os commits de Evidence e de `SourceSyncState`/`BackfillProgress`. Após reopen, nenhum cenário produziu o estado proibido `Evidence não durável / progresso avançado`.

Antes desta proposta de Decision, um probe no Go module proxy em Go 1.26.6 confirmou em 2026-08-19:

```text
github.com/ncruces/go-sqlite3@latest = v0.35.3
turso.tech/database/tursogo@latest   = v0.7.2
```

O probe foi executado em workflow descartável com roteamento explícito `[self-hosted, Linux, X64]` no runner `LuigiCachyOS` e removido sem merge após a coleta da Evidence.

A documentação upstream do `ncruces/go-sqlite3` declara o driver `database/sql` como cgo-free e informa que ele executa uma build Wasm do SQLite, traduzida para Go. Ao mesmo tempo, o módulo substitui a SQLite OS Interface/VFS por uma implementação Go própria, com diferenças documentadas sobretudo em file locking e suporte a WAL. Cada conexão também executa em ambiente Wasm próprio e possui custo de memória correspondente.

Logo, escolher `ncruces/go-sqlite3` **não** equivale a assumir que seu VFS possui comportamento idêntico ao VFS nativo do SQLite em toda plataforma. O que foi suportado pela Evidence do Limiar é a combinação concreta testada em Linux X64; locking/WAL em outras plataformas continuam gate explícito.

A matriz oficial de compatibilidade do Turso declara compatibilidade SQLite ainda parcial em pontos da query language e documenta `PRAGMA synchronous` somente para `OFF` e `FULL`. Isso não torna Tursogo incorreto: ele passou nossos contracts. A escolha abaixo prefere, para o banco novo, executar a engine SQLite via ncruces e manter explícita a superfície própria do Go VFS, em vez de carregar por inércia a reimplementação SQLite do storage legado.

Proposal de origem:
`docs/evolution/proposals/PROPOSAL-STO-001-local-sqlite-and-evidence-storage.md`.

## Decision proposta

Se este ADR for aceito, passam a valer os contratos abaixo para o **novo banco da rebaseline**.

### 1. Engine do novo banco: ncruces/go-sqlite3

O novo storage local usa:

```text
github.com/ncruces/go-sqlite3 v0.35.3
```

através de `database/sql`.

A versão deve permanecer pinada no `go.mod` de produção quando a implementação for autorizada. Atualizações futuras seguem a política normal de dependências do `AGENTS.md`: versão estável, changelog/docs oficiais, compatibilidade e testes proporcionais ao risco.

Esta Decision não declara `v0.35.3` permanente. Ela escolhe o baseline testado atual.

### 2. Baseline operacional conservadora

Inicialmente o novo banco usa uma única conexão lógica:

```text
SetMaxOpenConns(1)
SetMaxIdleConns(1)

PRAGMA journal_mode=WAL
PRAGMA synchronous=FULL
PRAGMA foreign_keys=ON
PRAGMA busy_timeout=5000
```

`cache_size` não entra no baseline.

A limitação inicial a uma conexão reduz concorrência implícita, simplifica ownership e limita o custo por conexão da implementação escolhida. Ela também corresponde à configuração concretamente exercitada pelos contracts atuais. Não é uma inferência de que o driver exija globalmente uma única conexão.

Aumentar o número de conexões exige Evidence de necessidade e revalidação de locking/ordering, mas não exige automaticamente novo ADR se não mudar autoridade ou durabilidade.

### 3. Evidence precede state/progress também no boundary físico

A implementação deve preservar explicitamente:

```text
Evidence durável / progresso antigo     -> replay permitido
Evidence durável / progresso novo       -> normal
Evidence não durável / progresso antigo -> falha segura
Evidence não durável / progresso novo   -> proibido
```

Para operações em commits separados, a ordem mínima é:

```text
commit Evidence
      ↓
commit SourceSyncState ou BackfillProgress
```

Uma transação única pode ser usada quando o boundary concreto permitir, desde que preserve a mesma semântica e não misture authorities distintas.

O fato de Evidence e progresso residirem no mesmo arquivo físico não os transforma na mesma autoridade.

### 4. Banco novo side-by-side; legado não é migrado in-place

O novo schema nasce em arquivo próprio.

O banco Tursogo legado deve permanecer preservado e tratado como fonte histórica/read-only durante importação e rollback. Não alterar o arquivo legado in-place para fazê-lo parecer o novo schema.

A política, ferramenta e ledger de importação ficam fora deste ADR.

### 5. Migrations SQL versionadas são a única autoridade de schema do banco novo

A evolução estrutural do novo banco usa migrations SQL versionadas e auditáveis.

Não manter no caminho normal do novo banco uma segunda autoridade que compare colunas em runtime e aplique `ALTER TABLE` ad hoc para “consertar” o schema.

Rotinas de compatibilidade dinâmica podem existir somente no boundary explícito de leitura/importação do legado.

### 6. database/sql é detalhe de infraestrutura, não authority global

O novo storage não deve expor um `*sql.DB` global como mecanismo de ownership.

Consumidores dependem de capabilities/repositories estreitos, por exemplo para:

- append de Evidence;
- `SourceSyncState`;
- `BackfillProgress`;
- futuras projeções duráveis autorizadas.

Os nomes e packages concretos continuam detalhe de implementação.

O antigo padrão `DB() *sql.DB + DBWriter global` não é carregado para a nova arquitetura apenas por existir.

### 7. Storage de Evidence permanece codec-agnostic

A camada de persistência deve aceitar payload de Evidence como bytes opacos acompanhados por metadata explícita de formato/versão suficiente para reconstrução futura.

Este ADR **não decide**:

- schema SQL final de `EvidenceRecord`;
- tipo físico de `EvidenceRecord.id`;
- precisão física final de timestamps;
- índices de domínio;
- codec do envelope Telegram;
- identidade/deduplicação de Evidence.

Esses itens devem ser decididos antes de a implementação correspondente virar schema canônico.

### 8. SourceSyncState e BackfillProgress permanecem authorities separadas

Mesmo no mesmo SQLite:

```text
Live sync -> SourceSyncState
Backfill  -> BackfillProgress
Ambos     -> Evidence append-only
```

`LastMessageID`, conclusão de backfill ou posição histórica não podem substituir `pts/qts/seq/channel pts` como autoridade de live sync.

### 9. Sessão MTProto e segredos ficam fora desta Decision

Este ADR não exige que sessão MTProto, credenciais ou outros segredos residam no novo banco.

Esses dados têm boundary de segurança próprio e serão decididos separadamente.

### 10. O banco legado continua legível pelo caminho de compatibilidade necessário

Aceitar este ADR não significa remover imediatamente Tursogo do repositório.

Enquanto importação/rollback do legado depender dele, o driver pode permanecer em um caminho explicitamente legado. A remoção da dependência histórica só ocorre quando o substituto e o processo de migração estiverem validados.

## Consequências

### Positivas

- o novo storage executa a engine SQLite via ncruces, enquanto as diferenças do Go VFS permanecem explícitas e gated por plataforma;
- runtime normal continua sem requisito de CGO;
- `database/sql` continua disponível sem virar authority global;
- `WAL + FULL` possui contrato explícito e foi exercitado na combinação Linux X64 testada;
- Evidence/state/progress possuem ordering físico verificável no ambiente exercitado;
- o banco histórico não é colocado em risco por migração in-place;
- migrations deixam de competir com “schema repair” dinâmico.

### Negativas

- `ncruces/go-sqlite3` é uma dependência estrutural pre-v1 e precisa permanecer pinada/testada;
- sua VFS em Go e a execução por conexão possuem características próprias de memória/locking;
- uma conexão pode limitar throughput futuro e precisará de Evidence antes de tuning;
- coexistência temporária com Tursogo aumenta a superfície durante migração;
- o schema físico de Evidence ainda precisa de outra Decision/gate antes de produção.

## Alternativas rejeitadas

1. **Manter Tursogo como default apenas por continuidade** — rejeitado como critério; o ADR 001 legado está em `SUPERSEDE` e continuidade não cria autoridade.
2. **Declarar Tursogo incorreto** — rejeitado; ele passou os contracts funcional, crash e race avaliados.
3. **Usar `synchronous=NORMAL` por compatibilidade com o legado** — rejeitado como baseline porque o upstream Turso não o documenta como contrato suportado, enquanto `FULL` é documentado e testado.
4. **Migrar o banco antigo in-place** — rejeitado por aumentar o risco sobre Evidence histórica e rollback.
5. **Continuar expondo `*sql.DB` global** — rejeitado porque topologia/ponteiro não define authority ou ownership.
6. **Adicionar ORM** — rejeitado por ausência de necessidade demonstrada.
7. **Fragmentar imediatamente todo estado em múltiplos bancos** — rejeitado por complexidade não justificada; separar authorities não exige separar arquivos físicos.

## Relação com ADRs legados e atuais

Se aceito:

- ADR 001 deixa de orientar o **novo** storage; Tursogo permanece somente onde explicitamente necessário ao legado/importação;
- ADR 003 não fornece authority ao novo writer; o fan-in DBWriter legado continua sem promoção automática;
- ADR 010 tem seu princípio de migrations retido, mas a authority do novo schema passa a ser migrations SQL versionadas sem repair concorrente;
- ADRs 016 e 017 continuam definindo os contracts de Evidence/sync/recovery;
- se o ADR 018 vier a ser aceito, este storage deve também sustentar seus boundaries de Source Admission.

Este ADR não muda o status dos ADRs legados no registry por si só; a limpeza final do registry pode ser feita quando a implementação/migração correspondente existir.

## Evidência de suporte

- `EXP-LIMIAR-002-storage-durability.md`;
- `EXP-LIMIAR-003-backfill-live-authority-separation.md`;
- `F-STO-001-tursogo-synchronous-compatibility.md`;
- `F-STO-002-process-crash-preserves-evidence-before-progress.md`;
- `PROPOSAL-STO-001-local-sqlite-and-evidence-storage.md`;
- probe `@latest` executado no Go module proxy em 2026-08-19;
- documentação oficial de `ncruces/go-sqlite3` sobre driver cgo-free, engine SQLite/Wasm, Go VFS, memória, locking/WAL e concorrência;
- matriz oficial de compatibilidade do Turso para SQLite/PRAGMAs.

## Gates antes de produção

Mesmo se este ADR vier a ser aceito, a implementação de produção continua condicionada a:

1. decidir o schema físico mínimo e versionado de Evidence, incluindo ID e timestamps;
2. preservar os contracts de `Evidence -> SourceSyncState/BackfillProgress` no storage real;
3. integrar os contracts dos ADRs 016/017 e, se aceito, ADR 018;
4. testar crash/restart da integração real, não somente o harness isolado de storage;
5. testar importação side-by-side em cópia real do banco legado;
6. testar backup/restore do schema final;
7. manter gates separados de `CGO_ENABLED=0 go test` e `go test -race`;
8. revalidar locking/WAL em cada plataforma oficialmente declarada como suportada;
9. não alegar resistência a power-loss durante `fsync` sem Evidence específica.

O upstream do ncruces possui testes em Linux ARM64, mas isso não é Evidence de integração do Limiar nem valida automaticamente o ambiente Android/Termux/PRoot do `LuigiTablet`. A Evidence física deste ciclo continua sendo Linux X64 no `LuigiCachyOS`; qualquer suporte ARM64 do Limiar deve ter gate próprio e roteamento explícito.

## Limite epistemológico

Os testes atuais suportam process crash nos boundaries observáveis de commit em Linux X64. Eles não simulam queda física de energia durante `fsync`, corrupção do dispositivo, bugs de filesystem ou equivalência automática em toda plataforma.

A escolha de `ncruces/go-sqlite3` é baseada em executar SQLite, preservar runtime cgo-free e nos contratos concretamente exercitados, aceitando explicitamente que o Go VFS continua uma superfície de compatibilidade própria. Não se baseia em benchmark isolado nem em falha do Tursogo.

## Escopo da proposta

`Status: Proposed` significa apenas que a Decision está pronta para avaliação.

Mergear este documento, aprovar a PR ou dizer `continue` não promove o ADR para `Accepted`.

Uma eventual aceitação exige instrução explícita do mantenedor e os metadados definidos em `AGENTS.md`.
