# PROPOSAL-STO-002 — Boundary de sessão MTProto em arquivo hardened

Authority: Non-authoritative
Status: Ready

## Problema

A sessão MTProto é material de credencial sensível e possui autoridade distinta de Evidence, sync state e peer cache. O runtime legado persiste sessão por um mecanismo próprio, enquanto a rebaseline ainda não possui uma Decision aceita para o boundary final dessa credencial.

F-STO-006 separou explicitamente sessão MTProto de peer cache. EXP-LIMIAR-018 rejeitou `github.com/gotd/td/session.FileStorage` v0.161.0 **as-is** como boundary final no ambiente Linux observado porque um arquivo preexistente `0644` permaneceu `0644` após `StoreSession` e o caminho upstream de escrita não fornece, por si só, a publicação hardened exigida para tratar a sessão como segredo durável.

EXP-LIMIAR-019 demonstrou que uma camada mínima baseada em arquivo consegue corrigir essas insuficiências no escopo Unix/intra-processo observado sem introduzir banco ou mecanismo distribuído adicional.

## Evidence

- F-STO-006 — sessão MTProto e peer cache possuem autoridades distintas;
- EXP-LIMIAR-018 — `session.FileStorage` as-is foi `Rejected` como boundary final;
- EXP-LIMIAR-019 — camada experimental hardened foi `Supported` no escopo testado;
- PROPOSAL-STO-001 — sessão permanece fora do SQLite de Evidence e deve ser decidida separadamente.

No EXP-LIMIAR-019, o gate Linux X64 com race detector confirmou simultaneamente:

- arquivo preexistente `0644` termina `0600`;
- publicação usa temporário no mesmo diretório, `Sync`, `Rename` e sync do diretório;
- duas instâncias no mesmo processo, coordenadas pelo mesmo registry por path, deixam um payload final completo;
- não ficam temporários após publicação bem-sucedida;
- o race detector permaneceu verde.

A Evidence **não** cobre ACL/replace no Windows, coordenação entre processos, power-loss real, criptografia em repouso, integração final com `gotd.Client` nem política operacional de backup/restore da credencial.

## Proposta

### 1. Sessão MTProto permanece fora do SQLite de Evidence

Não colocar a sessão no banco SQLite novo apenas por conveniência de persistência.

Evidence, sync state e backfill pertencem ao storage operacional definido pela linha STO-001. A sessão é segredo de autenticação e deve possuir boundary próprio, path próprio e lifecycle próprio.

### 2. Estratégia candidata: arquivo único hardened por identidade de sessão

Adotar como candidato de produção a menor estratégia já sustentada pela Evidence:

- um arquivo de sessão por identidade/configuração que realmente compartilhe a credencial;
- temporário criado no mesmo diretório;
- permissão privada aplicada antes da escrita quando a plataforma oferecer semântica POSIX;
- escrita completa + `fsync`/equivalente antes da publicação;
- publicação por replace/rename atômico quando suportado pelo contrato da plataforma;
- reafirmação de permissões privadas no destino;
- sync do diretório quando a plataforma fornecer semântica aplicável;
- coordenação intra-processo compartilhada por path para impedir writers concorrentes dentro do mesmo processo.

A implementação de produção não precisa reutilizar literalmente o harness do EXP-LIMIAR-019. O harness demonstra propriedades; não é API nem design final.

### 3. Fail-closed para proteção da credencial

Se a implementação não conseguir estabelecer as proteções exigidas para a plataforma suportada, a inicialização/escrita da sessão deve falhar explicitamente em vez de degradar silenciosamente para um arquivo mais permissivo.

Erros não podem incluir o conteúdo da sessão.

### 4. O lock é por path e compartilhado no processo

Clients independentes no mesmo processo que apontem para o mesmo arquivo precisam compartilhar o mesmo domínio de exclusão mútua.

Um mutex privado por instância não satisfaz esse contrato.

A implementação deve evitar crescimento global sem limite: o mecanismo de ownership/lifecycle dos locks precisa ser explícito e proporcional ao número real de sessões configuradas.

### 5. Multiprocesso não entra no contrato inicial

Não introduzir file lock cross-process, daemon de sessão, SQLite dedicado ou outro mecanismo apenas por antecipação.

Enquanto o deployment suportado não exigir writers de processos diferentes sobre a mesma sessão, coordenação multiprocesso permanece fora do contrato. Se esse requisito aparecer, deve ser investigado e testado separadamente antes de ampliar a implementação.

### 6. Suporte de plataforma é explícito

A Evidence atual autoriza recomendar o boundary somente para plataformas cujo contrato de permissões e replace seja verificado.

Linux/Unix observado possui Evidence experimental positiva. Windows **não** deve ser declarado suportado para esse boundary apenas porque a matriz geral do repositório ficou verde: os asserts relevantes do experimento foram pulados nessa plataforma.

Antes de habilitar Windows como plataforma suportada para session storage, deve existir Evidence específica de ACL privada e semântica segura de replace/recovery.

### 7. Backup de segredo não é automático

O arquivo de sessão não entra automaticamente no mesmo backup do banco de Evidence.

Backup/restore de credencial precisa de política separada que trate exposição, retenção, cópias antigas e revogação. Até essa política existir, nenhuma rotina genérica deve copiar a sessão por conveniência.

### 8. Integração deve preservar a interface estreita do gotd

A implementação candidata deve satisfazer `session.Storage` (`LoadSession`/`StoreSession`) sem expor o mecanismo de arquivo ao restante do domínio.

Nenhum consumidor deve depender de path, formato interno dos bytes ou detalhe de publicação.

## Alternativas

### `gotd/session.FileStorage` as-is

Rejeitada para o boundary final pela Evidence do EXP-LIMIAR-018. Pode continuar útil como referência de interface, não como implementação final sem hardening.

### SQLite para sessão

Não selecionado. Não existe Evidence de que um banco resolva um requisito demonstrado melhor do que a camada de arquivo mínima, e PROPOSAL-STO-001 já separa credencial do SQLite de Evidence. Introduzir outro schema/engine para um único blob de sessão adicionaria complexidade sem necessidade demonstrada.

### Storage remoto de segredo

Não justificado no estágio atual. Vault/KMS/serviço remoto introduziria disponibilidade, autenticação e operação distribuída sem requisito demonstrado.

### Criptografia local adicional

Desejável apenas se houver modelo de ameaça e key management que façam a criptografia acrescentar proteção real. Criptografar o arquivo com uma chave armazenada ao lado dele não deve ser tratado como ganho automático de segurança.

## Riscos e limitações

- `0600` não descreve ACL Windows;
- `rename`/replace possui diferenças entre plataformas;
- `fsync` reduz uma classe de risco, mas EXP-LIMIAR-019 não é teste de power-loss físico;
- lock intra-processo não evita writer concorrente externo;
- permissões de filesystem não protegem contra comprometimento do mesmo usuário/host;
- sessão comprometida pode permitir autenticação como a conta associada;
- política de backup/restore e revogação permanece aberta.

## Gates antes de produção

1. ADR explícito para o boundary de sessão, com plataforma(s) suportada(s) e propriedades mínimas;
2. aceitação explícita do mantenedor antes de integrar a implementação ao runtime;
3. implementar `session.Storage` em pacote de produção sem copiar autoridade do harness de teste;
4. testes de regressão para arquivo novo, arquivo preexistente permissivo, overwrite, cancelamento/erro e writers intra-processo;
5. `go test -race` no caminho real;
6. integração real com `gotd.Client`, demonstrando reopen/reuse da sessão sem reautenticação indevida;
7. confirmar que logs/erros não expõem bytes da sessão;
8. Evidence específica antes de declarar Windows suportado;
9. reabrir investigação se o deployment passar a exigir coordenação multiprocesso ou backup automatizado da credencial.

## Recomendação

Redigir um ADR **Proposed** estreito que escolha um arquivo hardened como boundary inicial de sessão MTProto para o ambiente Unix/Linux suportado, mantendo Windows, coordenação multiprocesso, criptografia adicional e política de backup fora do contrato até Evidence específica.

O ADR não deve autorizar implementação automaticamente. Pela constituição do projeto, `Proposed -> Accepted` exige aceitação explícita e verificável do mantenedor antes de qualquer mudança arquitetural permanente no runtime.
