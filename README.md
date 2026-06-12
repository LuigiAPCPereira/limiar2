<div align="center">
  <h1>🚀 Limiar</h1>
  <p><strong>Pipeline de Monitoramento e Processamento de Promoções do Telegram</strong></p>
  <p>
    <a href="#o-que-é">O que é</a> •
    <a href="#como-funciona">Como Funciona</a> •
    <a href="#quickstart">Quickstart</a> •
    <a href="#comandos-da-cli">CLI</a> •
    <a href="#configuração">Configuração</a> •
    <a href="#regras-do-projeto">⚠️ Regras do Projeto</a>
  </p>
</div>

---

O **Limiar** transforma o ruído dos canais de ofertas do Telegram em dados estruturados. Canais promocionais são uma excelente fonte de ofertas, mas sofrem com duplicidade, formatos de texto bagunçados e falta de estruturação.

O Limiar resolve isso através de um **Pipeline de Dados** construído em **Go** (Golang), que se conecta nativamente ao Telegram usando a tecnologia de _userbot_ (MTProto), monitora seus canais favoritos, extrai os links/preços/cupons e os disponibiliza em tempo real.

> [!NOTE]  
> Atualmente estamos finalizando as Fases 1 (Coleta) e 2 (Processamento), que rodam num orquestrador unificado (`limiar run`) e utilizam um banco local SQLite embeddado (`Tursogo`).

## 🛠 Como Funciona

```mermaid
flowchart LR
    A[Telegram] -- "MTProto" --> B[Collector]
    B -- "raw_messages" --> DB[(Tursogo .db)]
    DB -- "polling" --> C[Processor]
    C -- "Normalização\nClassificação" --> DB
    DB -- "processed_messages" --> D[Limiar API/Dashboard]
```

1. **Collector:** Autentica como um usuário (não bot), faz download de mensagens antigas (backfill) e fica escutando em tempo real (livestream). Salva a versão bruta no banco de dados.
2. **Processor:** Pega os payloads brutos, extrai URLs, detecta preços e cupons, categoriza se a promoção acabou, deduplica produtos iguais em canais diferentes, e gera os *processed_messages*.
3. **API & Dashboard:** Entrega os dados formatados (REST) e atualizações ao vivo (Server-Sent Events) para que o *Limiar Frontend* mostre a mágica acontecendo.

## ⚡ Quickstart

O Limiar não requer instalações complexas. Apenas o Go (versão 1.22+) instalado na máquina.

```bash
# 1. Compile o projeto
go build -o limiar ./cmd/limiar-collector

# 2. Defina suas credenciais do Telegram (AppID e APIHash)
export LIMIAR_APP_ID="123456"
export LIMIAR_API_HASH="sua_hash_secreta"

# 3. Faça o login pela primeira vez (interativo)
./limiar auth

# 4. Adicione um canal para monitorar
./limiar channels add "nome_do_canal"

# 5. Rode o orquestrador! (Collector + Processor + Dashboard)
./limiar run
```

## 💻 Comandos da CLI

O executável possui uma suíte de comandos interativos para gerenciamento:

- **`limiar auth`**: Realiza o login (pede telefone, código via SMS/App e senha 2FA). Só precisa rodar uma vez.
- **`limiar channels list`**: Lista quais canais estão sendo observados.
- **`limiar channels add <username>`**: Passa a escutar aquele canal.
- **`limiar channels remove <username>`**: Deixa de observar.
- **`limiar run`**: É onde a magia acontece. Inicia as goroutines do pipeline e expõe os endpoints HTTP e métricas.

## ⚙️ Configuração

Você pode usar variáveis de ambiente ou colocar um arquivo `.env` na raiz do projeto.

| Variável | Descrição | Padrão |
|----------|-----------|---------|
| `LIMIAR_APP_ID` | Telegram API ID (Obrigatório) | - |
| `LIMIAR_API_HASH` | Telegram API Hash (Obrigatório) | - |
| `LIMIAR_DB_PATH` | Caminho para o banco local | `./limiar.db` |
| `LIMIAR_LOG_LEVEL` | Verbosiade do log (`debug`, `info`, `warn`, `error`) | `info` |
| `LIMIAR_LOG_FORMAT` | Estilo do Log (`pretty`, `json`, `text`) | `pretty` |

> [!TIP]  
> Para desenvolvimento local, recomendamos usar `LIMIAR_LOG_FORMAT=pretty` para ver as mensagens chegarem coloridas no terminal com emojis indicativos!

## ⚠️ Regras do Projeto (Para Contribuidores e IAs)

> [!CAUTION]  
> Este repositório é governado por regras estritas documentadas no [AGENTS.md](file:///home/projetos/Projetos/Limiar2/AGENTS.md). **A LEITURA É OBRIGATÓRIA ANTES DE QUALQUER COMMIT.**

**Alguns dos invariantes do sistema:**
- **Nenhum ORM permitido:** Todo acesso a dados é via SQL explícito em `internal/storage`.
- **Closed Stack:** Só dependemos do `gotd/td` para Telegram, `tursogo` para o DB e `cobra/viper` pra CLI. Não instale novos pacotes levianamente.
- **Raw is the Truth:** O Collector nunca muta dados recebidos. A tarefa do Processor é criar cópias processadas.

Consulte a pasta `docs/` para mergulhar nos *Architecture Decision Records* (ADRs) e nas especificações de negócio (`docs/PRODUCT_BRIEF.md`, `docs/ARCHITECTURE.md`).
