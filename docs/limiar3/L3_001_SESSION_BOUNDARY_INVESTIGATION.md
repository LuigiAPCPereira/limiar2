# L3-001 — investigação de sessão MTProto e boundary Telegram

**Estado epistemológico:** investigação concluída como recomendação técnica / Proposal, não como Decision.

**Origem:** relatório produzido em outro chat e fornecido pelo mantenedor nesta conversa, ancorado em docs/limiar-3-foundation-20260922@8c059f10245a5ca248862becfbb00033300200a8.

**Limite desta internalização:** o HEAD e as fontes locais principais foram reabertos nesta conversa. As citações externas do relatório original sobre Telegram, gotd, Redis, Vault, SQLite, MCP e CloudHSM não foram reconsultadas aqui. A tarefa L3-001A deve verificá-las antes de usar detalhes externos como premissa de implementação.

## 1. Conclusão da investigação

A recomendação de L3-001 para o primeiro slice implementável é:

- um único TelegramRuntime autoritativo por identidade lógica de sessão;
- o runtime possui o gotd.Client e o lifecycle da integração Telegram;
- o SessionStore é privado ao boundary Telegram;
- MCP realtime e collector recebem capabilities estreitas, sem receber gotd.Client, tg.*, bytes de sessão, storage da sessão ou acesso direto ao SQLite;
- sessão, peer/access-hash state, update/recovery state, Evidence e autenticação MCP são autoridades distintas;
- a implementação inicial pode permanecer single-process; desacoplamento por contrato não implica microserviços;
- o backend inicial recomendado para a sessão é arquivo local endurecido em Unix/Linux, reaproveitando EXP-LIMIAR-019 como Evidence, sem aceitar automaticamente o ADR 023;
- a primeira capability Telegram deve ser read-only, bounded e cancelável, adequada ao MCP realtime;
- live updates/recovery do collector só devem ser ativados quando puderem atravessar Source Admission e respeitar os invariantes duráveis dos ADRs 016–018.

A recomendação permanece condicionada à investigação aprofundada de MTProto + gotd em L3-001A.

## 2. Boundary recomendado

    Config / secret references
              |
              v
       TelegramRuntime
         /         \
    SessionStore   gotd.Client
                      |
                   MTProto
                      |
                   Telegram
                  /        \
                 /          \
    Realtime query       Collector source
    capability           capability
         |                    |
         v                    v
    MCP Telegram         Source Admission
    realtime                  |
                               v
                            Evidence

Regras:

1. Sessão pertence ao boundary Telegram. MCP, collector e Evidence não são owners da credencial.
2. gotd/MTProto ficam encapsulados. Consumers não dependem dos tipos do SDK por conveniência.
3. MCP realtime consulta a fonte diretamente, sem usar Evidence como proxy obrigatório.
4. Consulta realtime não vira Evidence automaticamente. Para integrar o corpus durável, a observação precisa atravessar Source Admission.
5. MCP e collector têm lifecycles/failure domains distintos, mesmo compartilhando a mesma integração Telegram.
6. Topologia física é posterior. A capability deve permitir separação futura sem pagar antecipadamente por RPC, service discovery ou filas.

## 3. Ownership inicial

| Ativo / capability | Owner recomendado | Consumidores permitidos | Não deve acessar |
| --- | --- | --- | --- |
| Blob de sessão MTProto | Telegram boundary via SessionStore | gotd dentro do adapter/runtime | MCP, collector, Evidence |
| api_id / api_hash | configuração/secret input do Telegram runtime | bootstrap do runtime | ferramentas MCP |
| gotd.Client / tg.* | adapter/runtime Telegram | implementação interna | contracts da aplicação |
| Peer/access-hash state | capability/cache de peer | Telegram boundary | SessionStore como autoridade |
| Update/recovery state | acquisition/update subsystem | caminho do collector | MCP realtime |
| Evidence | Evidence store | Source Admission/collector | MCP realtime direto |
| MCP identity/tokens | transport/auth do MCP | servidor MCP | Telegram runtime |
| Realtime query | capability Telegram | MCP realtime | acesso bruto ao SDK |
| Live/recovery | capability Telegram/acquisition | collector | MCP realtime |

## 4. SessionStore recomendado para o primeiro slice

### 4.1 Escolha proposta

Candidato inicial: arquivo local endurecido, Unix/Linux, uma sessão por identidade configurada, separado do SQLite de Evidence.

Essa escolha é uma Proposal de L3-001. Ela não promove ADR 023. O EXP-LIMIAR-019 continua Evidence com escopo limitado.

### 4.2 Propriedades mínimas esperadas

- diretório pai dedicado e privado, equivalente a 0700;
- arquivo final privado, equivalente a 0600;
- temp file no mesmo diretório/filesystem;
- escrita integral do blob opaco;
- sync do arquivo temporário;
- rename/replace atômico quando suportado;
- revalidação/endurecimento das permissões do destino;
- sync do diretório quando aplicável;
- serialização intra-processo por identidade/path;
- cleanup best-effort de temporários;
- ausência de bytes de sessão, OTP, senha, API hash e identificadores sensíveis em logs;
- destino inesperado (symlink, FIFO, device, diretório) tratado explicitamente e, por padrão, fail-closed;
- suporte inicialmente declarado apenas onde houver Evidence;
- nenhuma alegação de coordenação multiprocesso sem mecanismo e teste próprios.

### 4.3 O que o relatório rejeita como default inicial

- sessão dentro do SQLite de Evidence;
- Redis apenas por performance;
- Postgres/DB remoto sem requisito multi-host;
- Vault/KMS/HSM como complexidade obrigatória antes de threat model;
- memória como solução persistente;
- múltiplos clients independentes usando o mesmo blob apenas porque MCP e collector são consumers diferentes.

Backends remotos ou cifrados permanecem substituições futuras possíveis se surgirem requisitos reais de centralização, multi-host, compliance ou proteção at-rest superior.

## 5. Lifecycle operacional recomendado

Estados conceituais:

    Absent -> Authenticating -> Active -> Restarted -> Active

    Active -> Invalid -> ReauthRequired -> Authenticating
    Active -> Compromised -> RemoteRevoke -> LocalDestroy -> ReauthRequired
    Active -> FatalStorageError -> fail closed

Regras propostas:

- not-found da sessão é distinto de erro de I/O/permissão;
- erro de storage não pode ser convertido silenciosamente em sessão ausente;
- StoreSession publica snapshots opacos completos, sem merge ou interpretação semântica pelo Limiar;
- apagar o arquivo local não deve ser tratado como revogação remota;
- backup de Evidence não inclui session material;
- eventual backup da sessão é outra cópia de credencial e precisa de política própria;
- não criar TTL local arbitrário sem requisito; autoridade sobre validade permanece no Telegram.

## 6. Capabilities mínimas propostas

Shape conceitual, não API final:

    RealtimeTelegram
      - ResolvePeer(ctx, ref)
      - RecentMessages(ctx, peer, query)

    CollectorTelegram
      - ResolvePeer(ctx, ref)
      - History(ctx, peer, cursor, limit)

Live updates/recovery devem ser desenhados levando em conta ADRs 016–018, porém não ativados antes de existir o boundary durável de Source Admission/Evidence necessário para impedir avanço de estado sem durabilidade.

Não criar agora:

- interface por método;
- facade que replique tg.Client;
- provider genérico para fontes hipotéticas;
- media capability em L3-002;
- contracts de domínio comercial;
- abstração de secret provider sem mais de uma necessidade concreta;
- API de mutação Telegram para o MCP realtime inicial.

## 7. Critérios propostos para L3-002

Antes de considerar L3-002 tecnicamente pronta para revisão, a investigação propõe demonstrar:

- G-A — owner único: uma autoridade runtime por session identity;
- G-B — restart real: login uma vez, shutdown, restart e query sem novo OTP;
- G-C — ausência != erro: not-found tipado separado de EACCES/I/O/corrupção/backend;
- G-D — publicação segura: falhas de escrita não publicam blob parcial e preservam snapshot anterior;
- G-E — containment: gotd/tg não vazam para contracts de MCP/collector;
- G-F — detachable boundary: MCP testável com fake da capability, sem Telegram real;
- G-G — cancellation/shutdown: cancelar request não cancela o runtime; cancelar root encerra tudo sem leaks conhecidos;
- G-H — zero segredo em observabilidade: logs/errors/fixtures/goldens não contêm material sensível;
- G-I — authorities separadas: session, peers, update state, Evidence e MCP auth não são colapsados;
- G-J — epistemologia preservada: ADRs 021–024 continuam Proposed e EXPs continuam Evidence;
- G-K — plataforma honesta: claim inicial limitada a plataformas realmente verificadas;
- G-L — qualidade: build/vet/test/race e gates estáticos aplicáveis na revisão exata.

Metas numéricas de latência/throughput não devem ser inventadas antes de baseline reproduzível.

## 8. Testes e Evidence necessários

### SessionStore

Cobrir pelo menos:

- arquivo ausente;
- permission denied;
- novo arquivo e overwrite com permissions endurecidas;
- falha em create/write/sync/rename/dir-sync;
- Store concorrente;
- cleanup de temp;
- cancellation onde aplicável;
- symlink/non-regular destination;
- ausência de segredos em logs/errors.

### Integração gotd real

Gap central ainda não provado pelo EXP-LIMIAR-019:

1. sem sessão -> autenticação explícita;
2. gotd chama StoreSession;
3. shutdown;
4. novo processo/runtime chama LoadSession;
5. conexão e query read-only funcionam sem novo login.

### Experimentos materiais sugeridos

- gotd real + hardened SessionStore + restart;
- crash/fault matrix do store;
- lifecycle de revogação;
- isolamento de backup de Evidence;
- concorrência bounded do MCP realtime;
- fairness MCP vs collector quando ambos existirem;
- backend cifrado somente se o threat model justificar;
- Windows somente se entrar no suporte pretendido;
- multiprocess somente se houver demanda real.

Cada experimento deve registrar também o que não prova.

## 9. Performance e observabilidade

Medir antes de otimizar:

- latência de LoadSession/StoreSession;
- frequência real de StoreSession;
- connect/reconnect;
- query realtime;
- history throughput;
- goroutines e heap;
- requests por consumer;
- rate/flood events;
- queue wait/backpressure;
- shutdown latency.

Não selecionar backend remoto por velocidade antes de observar frequência e custo reais das operações.

## 10. Relação com ADRs e Evidence existentes

- ADR 004: princípio histórico útil de sessão persistente, mecanismo legado não herdado automaticamente;
- ADR 023: Proposed; candidato de mecanismo, não Decision;
- EXP-LIMIAR-018: Evidence que rejeita FileStorage upstream as-is como boundary suficiente no cenário testado;
- EXP-LIMIAR-019: Evidence que sustenta hardened file em escopo Unix/intra-processo, sem provar gotd real, multiprocess, Windows, power-loss completo, encryption-at-rest ou backup/recovery;
- ADRs 016–018 Accepted: restringem o desenho de live updates/recovery pela exigência Evidence-first/Source Admission;
- ADRs 019/020 Accepted: Evidence/SQLite são autoridade distinta de session credential;
- ADRs 021/022/024: continuam Proposed.

## 11. Decisão ainda não tomada

Esta investigação não aceita:

- hardened file como arquitetura permanente;
- TelegramRuntime como nome/API final;
- um processo único como topologia permanente;
- Windows support;
- multiprocess/session sharing;
- backend remoto ou cifrado;
- modelo definitivo de peers/update state;
- contracts finais de gotd/MTProto;
- qualquer schema adicional.

Esses pontos só podem avançar conforme authority aplicável e Evidence suficiente.

## 12. Próximo gate

Antes de L3-002, executar L3-001A: relatório técnico profundo de MTProto + gotd.

Pergunta de gate:

> O comportamento real de MTProto e da versão de gotd usada pelo repositório sustenta a Proposal de L3-001 — um runtime autoritativo por session identity, capabilities estreitas, sessão privada ao boundary Telegram e compartilhamento in-process entre MCP/collector — ou exige alterações no boundary, lifecycle, recovery, peers, retries, concorrência ou sequência bottom-up?

Até esse relatório ser concluído, a recomendação acima é uma base de investigação consolidada, não um contrato de implementação autorizado.
