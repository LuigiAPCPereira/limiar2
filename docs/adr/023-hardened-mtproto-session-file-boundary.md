# ADR 023 — Boundary hardened de sessão MTProto em arquivo

Authority: Decision Record
Status: Proposed

## Contexto

A sessão MTProto é material de credencial sensível e precisa sobreviver a restart sem
reautenticação indevida. O registry de transição classifica o ADR 004 como
`RETAIN-PRINCIPLE / REWRITE`: a necessidade de sessão durável permanece, mas o mecanismo
de armazenamento deve ser redecidido.

A rebaseline separou a sessão MTProto do novo SQLite de Evidence. F-STO-006 distinguiu
sessão e peer cache como authorities diferentes. EXP-LIMIAR-018 rejeitou
`github.com/gotd/td/session.FileStorage` v0.161.0 **as-is** como boundary final no Linux
observado, pois um arquivo preexistente `0644` permaneceu permissivo após `StoreSession`.
EXP-LIMIAR-019 demonstrou, no escopo Unix/Linux e intra-processo observado, que uma camada
mínima consegue endurecer o destino para `0600`, publicar por temporário no mesmo
diretório com sync + rename, coordenar writers por path e permanecer race-clean.

PROPOSAL-STO-002 consolidou essa Evidence e recomenda um boundary de arquivo hardened
estreito, sem promover o harness experimental a implementação de produção.

## Decisão proposta

Se este ADR for aceito, o boundary inicial de sessão MTProto do novo runtime será um
arquivo hardened próprio, separado do SQLite de Evidence e exposto ao gotd apenas através
de `session.Storage` (`LoadSession`/`StoreSession`).

Para plataformas suportadas por esta decisão, a implementação deve preservar estas
propriedades mínimas:

1. um arquivo de sessão por identidade/configuração que compartilhe a mesma credencial;
2. temporário criado no mesmo diretório do destino;
3. proteção privada estabelecida antes da publicação e reafirmada no destino;
4. escrita completa seguida de sync do arquivo antes da publicação;
5. publicação por replace/rename com semântica adequada à plataforma;
6. sync do diretório quando a plataforma oferecer semântica aplicável;
7. coordenação intra-processo compartilhada por path entre instâncias independentes;
8. falha fechada quando as proteções exigidas não puderem ser estabelecidas;
9. erros e logs nunca incluem os bytes da sessão.

O mecanismo de locks deve possuir ownership/lifecycle explícito e não pode crescer sem
limite apenas porque paths antigos foram usados.

## Plataforma inicial

A decisão proposta autoriza apenas o escopo Unix/Linux coberto pela Evidence atual.

Windows não é declarado suportado por este ADR enquanto não existir Evidence específica
para ACL privada e semântica segura de replace/recovery. O fato de a matriz geral do
repositório executar em Windows não substitui esses testes, pois os asserts POSIX do
EXP-LIMIAR-019 foram pulados nessa plataforma.

## Fora do contrato inicial

Este ADR não decide nem exige:

- coordenação entre processos distintos;
- file lock cross-process;
- criptografia local adicional;
- KMS/Vault/storage remoto;
- backup automático da credencial;
- política de restore/revogação;
- peer cache no mesmo boundary;
- formato interno dos bytes da sessão;
- reutilização literal do harness do EXP-LIMIAR-019.

Se deployment, threat model ou requisitos operacionais introduzirem alguma dessas
necessidades, elas devem ganhar Evidence própria antes de ampliar o contrato.

## Relação com ADR 004

Enquanto este ADR estiver `Proposed`, o ADR 004 permanece apenas sob a interpretação
transitória `RETAIN-PRINCIPLE / REWRITE` definida em `docs/adr/README.md`; este documento
não o supersede ainda.

Se o ADR 023 for explicitamente aceito pelo mantenedor, ele substituirá a escolha concreta
de armazenamento do ADR 004, preservando somente o princípio histórico de que a sessão
deve ser durável e reutilizável após restart. Nesse momento, o ADR 004 deverá ser marcado
`Superseded` com referência ao ADR 023.

## Consequências esperadas se aceito

- sessão deixa de herdar lifecycle, backup e authority do banco de Evidence;
- `gotd.Client` continua dependente apenas da interface estreita `session.Storage`;
- falhas de proteção da credencial tornam-se erros explícitos, não degradação silenciosa;
- writers no mesmo processo e path compartilham o mesmo domínio de exclusão;
- suporte de plataforma torna-se explícito e sustentado por Evidence;
- o novo SQLite não ganha responsabilidade sobre um segredo apenas por conveniência.

O custo é manter um boundary adicional de persistência e testar suas garantias de
filesystem separadamente do banco de Evidence.

## Gates de implementação

Mesmo após eventual aceitação deste ADR, a implementação deve demonstrar antes de entrar
no runtime suportado:

1. testes para arquivo novo, arquivo preexistente permissivo e overwrite;
2. cleanup correto em falhas e ausência de temporários após sucesso;
3. writers intra-processo concorrentes deixando apenas payload final íntegro;
4. `go test -race` no caminho real;
5. integração com `gotd.Client` comprovando reopen/reuse da sessão sem reautenticação
   indevida;
6. logs e erros sem exposição dos bytes da sessão;
7. fail-closed quando permissões ou publicação segura não puderem ser garantidas;
8. nenhuma declaração de suporte Windows sem Evidence específica.

## Evidence relacionada

- F-STO-006 — separação de authority entre sessão MTProto e peer cache;
- EXP-LIMIAR-018 — `session.FileStorage` as-is `Rejected`;
- EXP-LIMIAR-019 — camada de arquivo hardened `Supported` no escopo observado;
- PROPOSAL-STO-001 — sessão fora do SQLite de Evidence;
- PROPOSAL-STO-002 — proposta direta deste ADR.

## Estado de autoridade

`Status: Proposed` não autoriza implementação arquitetural de produção.

A transição para `Accepted` exige aceitação explícita e verificável do mantenedor conforme
`AGENTS.md` e `docs/adr/README.md`.