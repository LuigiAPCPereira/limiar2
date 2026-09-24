# Registro de aceitação — pacote de fundação pré-L3-002

**Data local do mantenedor:** 2026-09-23  
**Escopo:** Limiar 3.0 — decisões de fundação anteriores à implementação de L3-002.

## Aceitação do mantenedor

Na conversa do projeto, após a conclusão de L3-001/A/B/C, o mantenedor aprovou o pacote de decisões proposto com uma correção de governança:

> ADRs exclusivos do Limiar 3 não continuam a numeração histórica da rebaseline; o Limiar 3 é uma reconstrução e seu namespace de ADRs recomeça em 001.

A aprovação cobre o pacote abaixo, sujeito aos Implementation Gates e experimentos definidos nos ADRs correspondentes.

## Pacote aceito

1. **Namespace próprio de ADRs Limiar 3**
   - `docs/limiar3/adr/`;
   - numeração reiniciada em `001`;
   - ADRs históricos da rebaseline permanecem como fontes históricas/Evidence conforme seus próprios estados.

2. **Toolchain**
   - Go **1.27.1** como baseline inicial aceita para L3-002, seguindo `Current Stable First`;
   - versões anteriores somente como fallback mediante incompatibilidade/regressão concreta.

3. **gotd**
   - `github.com/gotd/td v0.162.0` como pin inicial aceito para L3-002;
   - pin exato, nunca `latest` dinâmico;
   - upgrades posteriores seguem `Current Stable First` e gates proporcionais.

4. **Plataforma/deployment inicial**
   - Linux;
   - single-host;
   - single-process por `TelegramAuthorizationIdentity`;
   - sem claim inicial para Windows, multiprocess sharing ou network filesystem.

5. **Credential/session storage**
   - hardened local file;
   - separado de Evidence e peer state;
   - implementação privada satisfaz diretamente `gotd/session.Storage`;
   - fail-closed, publicação segura/atômica conforme contrato Linux aceito;
   - nenhuma criptografia/KMS/Vault obrigatória no primeiro threat model.

6. **Authorization/runtime lifecycle**
   - uma `TelegramAuthorizationIdentity` é a unidade de ownership;
   - exatamente um main `telegram.Client` por authorization identity;
   - gotd permanece owner de MTProto, reconnect, pools, DC migration, RPC e retries internos;
   - bootstrap é operação administrativa explícita;
   - steady-state nunca inicia login silenciosamente;
   - autorização ausente/incompatível/revogada falha fechada e exige ação administrativa;
   - semantic readiness exige autorização válida + identidade `self` esperada + runtime operacional.

7. **Bootstrap**
   - superfície administrativa separada do daemon normal;
   - QR-first como caminho operacional preferido;
   - phone/code/2FA como fallback controlado;
   - OTP, password, QR token e session bytes nunca entram em logs.

8. **Primeira capability**
   - read-only e bounded;
   - `ResolvePeer + History`;
   - resolução on-demand inicialmente;
   - sem persistent peer DB inicial;
   - `tg.*`, `InputPeer`, access hashes e session bytes permanecem dentro do Telegram boundary.

9. **Retry/FLOOD_WAIT**
   - nenhum retry framework genérico do Limiar;
   - nenhum `gotd/contrib/middleware/floodwait.Waiter` global na primeira fundação;
   - FLOOD_WAIT é classificado/propagado para política do workload.

10. **Deferred**
    - PFS não bloqueia L3-002;
    - updates/recovery/Source Admission entram na fatia collector;
    - media entra em fatia própria;
    - PGO, custom pools, unsafe, zero-copy, scheduler sofisticado e serviço/IPC só após Evidence.

## Limites da aceitação

Esta aceitação:

- **autoriza as Decisions documentais** correspondentes;
- **não inicia automaticamente a implementação** de L3-002;
- não autoriza login Telegram, OTP, criação de credencial, deploy ou migração;
- não promove ADRs históricos `021–024`;
- não remove os Implementation Gates;
- não transforma experimentos futuros em produção sem verificação.

Os ADRs do Limiar 3 devem usar este registro/commit como referência durável de aceitação.