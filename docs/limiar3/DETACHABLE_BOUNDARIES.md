# Limiar 3.0 — boundaries desacopláveis por construção

**Estado:** requisito arquitetural/documentação de direção do mantenedor. Não define topologia física, não aceita ADR Proposed e não autoriza implementação por si só.

## 1. Princípio

**Desacoplar não significa distribuir.** Primeiro separar authority, ownership, lifecycle, failure domain e contrato. Processo, binário, goroutine, socket ou deployment são decisões posteriores, justificadas por necessidade observável.

O Limiar 3 deve permitir que integrações estruturais sejam substituíveis ou separáveis sem obrigar reescrita do core. Isso se aplica especialmente a sessão/credencial MTProto, integração Telegram/MTProto, MCP, collector/Source Admission, Evidence/processing e API/frontend.

Esse princípio concretiza C-08 (topologia não define autoridade) e C-09 (complexidade exige Evidence proporcional) do `AGENTS.md`.

## 2. Telegram/MTProto como boundary

`gotd/td` e detalhes MTProto pertencem ao adapter Telegram, não ao core comercial do Limiar. O restante do sistema não deve depender diretamente, por conveniência, de `gotd.Client`, `tg.*`, `updates.Manager`, sessão serializada ou access hashes.

```text
Session Credential
       |
       v
Telegram / MTProto Adapter
       |
       +--> Realtime Query Capability
       +--> Update/Recovery Capability
       +--> History Capability
       +--> Peer Capability
       +--> Media Capability
```

Os nomes são ilustrativos. Não criar uma interface para cada método sem necessidade demonstrada.

### Sessão

A sessão pertence ao boundary Telegram/MTProto. Collector e MCP não são donos da credencial. Eles recebem capabilities já compostas. Se futuramente houver mais de um client/processo usando a mesma identidade, coordenação e política de segredo precisam ser explícitas; a topologia não é decidida aqui.

## 3. MCP desacoplável

O MCP é parte do produto, mas deve ser **detachable by construction**:

- Limiar pode operar coleta/Evidence/processing sem MCP;
- MCP realtime pode rodar no mesmo processo inicialmente e ser separado depois sem reescrever o domínio;
- trocar MCP por outra superfície não altera o core;
- MCP não conhece schema SQLite nem acessa `*sql.DB` diretamente;
- MCP não implementa regras comerciais canônicas;
- MCP nunca recebe bytes da sessão MTProto como dado de aplicação.

### 3.1 Telegram realtime tools — cedo

Consultam o Telegram em tempo real, sob demanda, através da capability Telegram/MTProto. Não dependem de a mensagem já existir no storage do Limiar.

Exemplos de intenção, sujeitos à API real e autorização: buscar histórico/mensagens, resolver canal/peer, obter mensagem por referência, explorar mídia e pesquisar padrões em mensagens recentes.

Essas ferramentas entram **antes da modelagem comercial** para permitir que ChatGPT e desenvolvimento investiguem o corpus real e descubram quais dados de promoção realmente existem, variam ou são ambíguos.

Uma resposta MCP realtime **não vira Source Evidence automaticamente**. Se uma observação deve integrar o corpus durável do Limiar, ela precisa atravessar o boundary de Source Admission apropriado.

### 3.2 Limiar data tools — depois

Quando projeções e modelo comercial existirem, o mesmo servidor MCP pode ganhar ferramentas sobre a `Query Service` do Limiar: ofertas, produtos/listings, comparações, provenance e outros dados processados.

As tools Telegram realtime e Limiar data podem coexistir no mesmo servidor MCP, mas possuem authorities distintas.

## 4. Dois consumidores, uma integração Telegram

```text
                         Telegram
                            |
                            v
                 Telegram / MTProto Adapter
                    /                 \
                   /                   \
       realtime query capability   update/history capability
                 |                         |
                 v                         v
          MCP Telegram realtime       Collector
                                           |
                                           v
                                    Source Admission
                                           |
                                           v
                                        Evidence
```

O collector produz Evidence sob contratos de durabilidade. O MCP realtime faz consulta exploratória sob demanda. Compartilhar uma integração não torna os lifecycles iguais.

## 5. MCP como instrumento de descoberta do domínio

O modelo de promoções não deve ser inventado antes de observar o corpus.

```text
Telegram realtime via MCP
        |
        v
investigação de casos reais
        |
        v
Findings sobre padrões, exceções e UNKNOWN
        |
        v
Proposal/Decision quando estrutural
        |
        v
Deterministic Findings / Promotion Interpretation
        |
        v
Product / Listing / Offer / Relations
```

Perguntas úteis: como preços aparecem; como cupons, Pix, cashback, frete e bundles variam; quando há múltiplos produtos; quais campos faltam; quais casos são ambíguos; como mídia aparece por canal/merchant.

Observações obtidas pelo MCP são material de investigação. Elas podem sustentar Findings documentais, mas não viram schema, regra canônica ou Decision automaticamente.

## 6. Consequência de topologia

A implementação inicial pode continuar single-process se essa for a menor solução suficiente. A arquitetura deve permitir separação futura, mas não deve pagar antecipadamente por RPC, service discovery, filas ou deployment distribuído sem requisito demonstrado.

**Desacoplável por contrato agora; separável fisicamente quando houver motivo.**
