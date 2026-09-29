# L3 ADR 005 — Runtime, Telegram, MCP e armazenamento agnósticos à plataforma

Authority: Decision Record — Limiar 3.0  
Status: Accepted  
Accepted-by: Mantenedor do Limiar  
Accepted-at: 2026-09-29  
Acceptance-reference: `c940373f46d2a522e3f3c0d74ab72ecaa0f28d1f`

## Contexto

A fundação L3-002 registrou Linux como plataforma/deployment inicial e a implementação transformou essa limitação operacional em restrição de compilação. Arquivos de runtime, query, bootstrap, ownership, observabilidade e, depois, o adapter MCP L3-003 receberam sufixo `_linux.go` e `//go:build linux`.

Essa propagação não corresponde à continuidade arquitetural do Limiar:

- o Limiar anterior possuía armazenamento de sessão em arquivo por `os.ReadFile`/`os.WriteFile` sem build tag de sistema operacional;
- o mesmo Limiar possuía `session.Storage` apoiado em banco;
- o Limiar 2 usava `TursoSessionStorage` sobre `storage.Repository`, também sem restrição de sistema operacional;
- o repositório histórico contém validação nativa de SQLite no Windows;
- `TelegramQuery`, lifecycle do gotd, observabilidade e o servidor MCP usam contratos e bibliotecas que não exigem Linux por natureza.

A escolha de um ambiente inicial de desenvolvimento, CI ou deployment não deve alterar silenciosamente o conjunto de plataformas em que o código do produto pode ser compilado.

## Decision

### 1. Neutralidade de plataforma é o padrão

O Limiar 3 é agnóstico ao sistema operacional por padrão.

Código de produto não recebe sufixo de plataforma nem build tag de sistema operacional apenas porque a primeira validação ocorreu em determinada plataforma.

Uma restrição como `//go:build linux`, `_windows.go` ou equivalente só é admitida quando:

1. o arquivo realmente depende de API, syscall ou semântica exclusiva daquela plataforma;
2. a especialização fica no menor adapter possível;
3. o contrato acima desse adapter permanece portável quando isso for tecnicamente viável;
4. a necessidade e o comportamento alternativo ficam documentados e testados de forma proporcional.

### 2. Runtime Telegram e MCP são portáveis

Os componentes abaixo são independentes de sistema operacional e devem ser compiláveis sem build tags de plataforma:

- ownership por `TelegramAuthorizationIdentity`;
- bootstrap administrativo;
- runtime e semantic readiness;
- `TelegramQuery`, paginação e tradução de erros;
- observabilidade;
- MCP Telegram realtime e seus testes;
- harness de integração Telegram, condicionado apenas pela build tag funcional `telegram_real`, não por Linux.

Identificadores, APIs e contratos externos permanecem como exigidos por Go, gotd e MCP.

### 3. Armazenamento de sessão mantém segurança sem definir o produto por Linux

O armazenamento local de sessão continua separado de Evidence e satisfaz diretamente `gotd/session.Storage`, mas sua implementação base deve usar primitivas portáveis da biblioteca padrão.

O primeiro contrato portável preserva:

- caminho canônico;
- coordenação intra-processo por caminho;
- rejeição de symlink e de alvo existente que não seja arquivo regular;
- temporário exclusivo no mesmo diretório;
- solicitação de modo privado `0600` onde o sistema operacional aplicar bits POSIX;
- escrita completa;
- `Sync` do arquivo antes da publicação;
- `Rename` para publicação atômica no filesystem local compatível;
- ausência de bytes de sessão em erros e logs;
- cancelamento respeitado antes de I/O relevante e durante aquisição da coordenação.

Não é requisito do contrato base consultar UID/GID via `syscall` nem falhar apenas porque uma plataforma não representa permissões como POSIX. Se um threat model futuro exigir ACL/ownership específico de uma plataforma, isso deve entrar como adapter estreito ou hardening adicional, sem tornar todo o Telegram boundary específico daquele sistema.

### 4. Single-host/single-process não significa Linux-only

As limitações iniciais de single-host e coordenação intra-processo continuam sendo limitações de lifecycle/concorrência, não de sistema operacional.

Multiprocess sharing, network filesystem e coordenação distribuída permanecem fora do primeiro contrato até existir necessidade comprovada.

### 5. Idioma humano

Nos arquivos alterados por esta correção, comentários, docstrings, mensagens operacionais, erros destinados a pessoas e documentação devem usar pt-BR por padrão.

Não traduzir identificadores de código, nomes de packages, APIs, campos JSON, nomes de tools MCP, valores de protocolo, comandos ou outros contratos técnicos cuja grafia seja parte da interface.

## Relação com decisões anteriores

Esta ADR é posterior e prevalece somente nos conflitos de plataforma com:

- **L3 ADR 001:** a menção a Linux como escopo do credential/session storage deixa de restringir a implementação base; os requisitos de separação, atomicidade, fail-closed e segurança permanecem, conforme refinados aqui;
- **L3 ADR 002:** a frase de deployment inicial Linux não autoriza build tags de plataforma no runtime; ownership, bootstrap, readiness e `TelegramQuery` permanecem vigentes;
- **L3 ADR 004:** não muda o boundary MCP, o read scope, o transporte loopback-only nem o trust model. Apenas torna explícito que o adapter MCP é código portável.

A referência histórica `DECISION_ACCEPTANCE_2026-09-23.md` continua preservada como Evidence do que foi registrado naquela data, mas sua cláusula de plataforma foi corrigida pela aceitação desta ADR.

## Gates de implementação

Antes de considerar `L3-PLAT-001` validada:

1. não deve restar `//go:build linux` em código de produto L3 que não use dependência Linux real;
2. runtime, query, bootstrap, ownership, observabilidade e MCP devem usar nomes neutros de plataforma;
3. o harness real deve depender apenas de `telegram_real`;
4. session storage deve compilar a partir de APIs portáveis e manter testes de atomicidade, cancelamento, symlink/non-regular e privacidade aplicável;
5. comentários/mensagens humanas alterados devem estar em pt-BR;
6. `go build ./...`, `go vet ./...`, `go test ./...` e `go test -race ./...` devem ser executados no toolchain aceito quando houver canal disponível;
7. validação em mais de um sistema operacional deve ser adicionada quando o canal de CI estiver disponível; ausência temporária desse canal permanece UNKNOWN, não PASS.

## Fora do escopo

Esta Decision não:

- cria suporte multiprocess/network filesystem;
- altera credenciais Telegram reais;
- muda o modelo de Evidence;
- altera o read scope MCP;
- cria listener público;
- cria OAuth/multi-user;
- autoriza merge ou deploy;
- exige renomear identificadores técnicos apenas por idioma.

## Consequências

O Limiar 3 volta a preservar a propriedade histórica de neutralidade de plataforma. A segurança de credenciais permanece um contrato do storage, e não uma justificativa para acoplar runtime, Telegram ou MCP ao Linux.

O custo é tornar explícitas as garantias realmente portáveis e separar, no futuro, qualquer hardening específico de plataforma no menor ponto possível. Esse custo é preferível a excluir plataformas inteiras por uma conveniência de implementação.
