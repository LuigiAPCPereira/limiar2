# EvolutionDocs — Processo de evolução do Limiar

Authority: Non-authoritative

EvolutionDocs registra como o Limiar aprende antes de transformar aprendizado em
arquitetura. Ele existe para impedir que pesquisa, experimento ou proposta seja
confundido com decisão.

---

## Estrutura

```text
docs/evolution/
├── README.md
├── findings/
├── experiments/
└── proposals/
```

Evidence pode ser incorporada ao Finding ou Experiment correspondente; não é necessário
criar um arquivo separado para cada observação.

Hypothesis normalmente pertence ao Experiment e também não precisa de diretório próprio.

---

## 1. Finding

Identificador recomendado:

`F-<ÁREA>-NNN`

Exemplos:

- `F-ING-001`
- `F-PROC-003`
- `F-ID-002`

Um Finding deve responder:

- o que foi observado;
- qual Evidence sustenta isso;
- qual é o impacto;
- o que ainda não sabemos.

Finding não prescreve automaticamente a solução.

---

## 2. Experiment

Identificador:

`EXP-LIMIAR-NNN`

Um experimento deve registrar:

### Hipótese

O que está sendo testado.

### Escopo

O que o experimento pode provar e, principalmente, o que ele não prova.

### Setup

Versões, configuração e fixtures necessárias para reprodução.

### Critério

Qual resultado confirma, rejeita ou deixa inconclusiva a hipótese.

### Resultado

Evidence produzida.

### Conclusão

Use somente:

- `Supported`
- `Rejected`
- `Inconclusive`

`Supported` significa que a hipótese sobreviveu ao experimento. Não significa que a
arquitetura foi aceita.

### Limitações

O que continua pendente.

Código experimental descartável pode viver fora do produto ou em área explicitamente
experimental.

Sem ADR Accepted, um experimento não pode alterar comportamento padrão de produção,
schema canônico, source of truth, API pública ou dependências obrigatórias.

---

## 3. Proposal

Identificador:

`PROPOSAL-<ÁREA>-NNN`

Uma Proposal transforma Findings e Experiments em uma recomendação.

Deve conter:

- problema;
- Evidence e Findings relevantes;
- alternativas;
- solução proposta;
- trade-offs;
- riscos;
- impacto de migração;
- pontos ainda abertos.

Status permitidos:

- `Draft`
- `Ready`
- `Withdrawn`

`Ready` significa pronta para decisão, não aceita.

EvolutionDocs não devem usar `Accepted`, `Canonical`, `Active` ou `Authoritative` como
status próprio. Esses termos só podem aparecer ao referenciar uma Decision verdadeira.

---

## 4. ADR

Quando uma Proposal exige uma decisão arquitetural durável, ela pode originar um ADR em
`docs/adr/`.

O ADR nasce com:

`Status: Proposed`

Somente o mantenedor pode autorizar a promoção para:

`Status: Accepted`

A aceitação deve ficar registrada de forma durável conforme `AGENTS.md` e
`docs/adr/README.md`.

Um ADR Proposed não autoriza produção arquitetural.

---

## 5. Fluxo

Fluxo completo quando necessário:

```text
Evidence
   ↓
Finding
   ↓
Hypothesis
   ↓
Experiment
   ↓
Proposal
   ↓
ADR Proposed
   ↓
aceitação explícita
   ↓
ADR Accepted
   ↓
Implementation
   ↓
Baseline / Architecture atualizadas
```

Nem toda mudança exige todos os passos.

Uma correção simples pode ir diretamente de Evidence para implementação. Mudanças de
durabilidade, identidade, sincronização, source of truth, segurança ou dependência
estrutural exigem evidência proporcional ao risco.

---

## 6. Autoridade

EvolutionDocs não são autoridade arquitetural.

Eles podem:

- contradizer uma decisão existente;
- demonstrar que uma decisão envelheceu;
- recomendar sua substituição.

A Decision vigente continua sendo a autoridade até ser explicitamente superseded.

---

## 7. Reprodutibilidade

Quando uma conclusão depender de comportamento externo, registre quando material:

- biblioteca, engine ou provider;
- versão ou model ID;
- data;
- configuração relevante;
- fonte primária consultada;
- comando ou procedimento do teste.

Evite afirmações como "latest", "rápido" ou "suportado" sem contexto reproduzível.

---

## 8. Critério de encerramento

Um EvolutionDoc pode ser encerrado sem virar ADR.

Resultados válidos incluem:

- hipótese rejeitada;
- experimento inconclusivo;
- Proposal retirada;
- problema resolvido localmente;
- decisão conscientemente adiada.

O processo existe para gerar conhecimento, não para fabricar arquitetura.
