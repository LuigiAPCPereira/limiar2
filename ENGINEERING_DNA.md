# Engineering DNA — entrada do Limiar 3.0

**Atenção: este arquivo é uma ENTRADA resumida, não a transcrição textual completa.** O Engineering DNA original **integral, byte a byte**, está preservado nesta branch em [`docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64`](docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64). Foi compactado apenas para transferência para o repositório: seu Git blob verificado é `ceafec243be87ae28ee2d9658e3912831fc55368`. Ao decodificar, deve produzir exatamente 37.531 bytes e SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`, idêntico ao anexo de origem do ChatGPT Project, **sem modificação de princípios**. O blob remoto e o blob calculado diretamente da fonte original foram confrontados e são idênticos. Esta apresentação compactada não é substituto legível do original: **descompactar para obter o documento de trabalho integral antes de mudança arquitetural substantiva**, em qualquer ambiente autorizado que tenha clone local e ferramentas de decodificação.

```bash
# Executar na raiz de um clone da branch, em ambiente local autorizado:
base64 -d docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64 | gzip -dc > /tmp/LIMIAR_ENGINEERING_DNA_FULL.md
sha256sum /tmp/LIMIAR_ENGINEERING_DNA_FULL.md
# Deve imprimir c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373
```

No Windows/ambiente sem `base64` e `gzip`, usar script local equivalente de Base64 decode + gzip descompressão e comparar SHA-256; não afirmar acesso às ferramentas sem verificar. Se um agente não puder descompactar, declarar a limitação e consultar o arquivo original do Project **somente se realmente acessível**. Não inventar sincronização entre Project e GitHub.

## Princípios operacionais de entrada

> Observe before assuming. Give every responsibility a clear owner. Keep boundaries narrow. Preserve unknowns as unknowns. Fail safely. Test behavior that matters. Measure before optimizing. Make the smallest commitment justified by evidence.

- Separar domínio, aplicação, transporte, persistência e integrações pelas responsabilidades e ciclos de vida reais; infraestrutura adapta-se a contratos internos quando necessário, sem DI framework por convenção.
- Sessão MTProto != peer cache != identidade do usuário != autorização MCP. Configuração != estado runtime. Evidence != interpretação derivada. API/frontend não duplicam regras comerciais.
- Código idiomático, simples, coeso e sustentável; não criar pastas/interfaces/frameworks apenas para aparentar organização; preservar legacy apenas onde for necessário para compatibilidade e migrar comprovadamente.
- Durabilidade antes de avanço de progresso, idempotência onde necessária, falha segura, segredo nunca em logs, limites de concorrência, cancelamento, backpressure e recovery com testes.
- Medir latência, throughput, CPU, memória, I/O e comportamento antes/depois onde relevante; não declarar ganho sem Evidence. Testar cenários reais e falhas, não apenas build.
- Implementar incrementalmente de baixo para cima por fatias completas quando estiver autorizado e o Implementation Gate passar; evitar ciclos infinitos de ADRs e abstrações especulativas.

Para particularização do Limiar 3.0 leia [`docs/limiar3/ENGINEERING_GUIDE.md`](docs/limiar3/ENGINEERING_GUIDE.md), [`docs/limiar3/START_HERE.md`](docs/limiar3/START_HERE.md), raiz `AGENTS.md` e `docs/DOCUMENTATION_AND_CONTINUITY.md` v2.0. **Em conflitos respeitar AGENTS, requisitos explícitos e ADRs Accepted**. Este arquivo e o guia não elevam sua própria autoridade nem concedem permissão para iniciar código, abrir acesso Telegram/MCP, merge ou deploy.

**Limite residual explícito:** o original está presente **compactado**, não como Markdown integral legível na raiz. Para declarar entrada documental finalizada, verificar que o próximo ambiente consegue recuperá-lo e comparar SHA; não dizer que já foi instalado/sincronizado em Codex, ChatGPT ou agendamentos por existir nesta branch.