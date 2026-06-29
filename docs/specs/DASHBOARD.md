# Limiar Dashboard — API Contract & Data Reference

Você vai gerar um dashboard para o pipeline Limiar, um agregador de promoções do Telegram.

## STACK RECOMENDADA

| Camada | Escolha |
|--------|---------|
| Build | Vite |
| Linguagem | TypeScript |
| UI | Tailwind CSS |
| Reatividade | Alpine.js (via npm `alpinejs`, **não CDN**) |
| Estado global | Alpine Stores (`Alpine.store()`) |
| Ícones | Lucide (via npm `lucide`, tree-shaking) |
| HTTP | Fetch wrapper tipado em `src/api.ts` |
| Gráficos | **Não.** É ferramenta de inspeção, não analytics |
| Router | **Não.** São 2 tabs, `x-show` resolve |

O output final é `dist/` (build do Vite). O Go embute a pasta inteira com `//go:embed all:dist` usando `embed.FS`.

> Se usar Alpine.js, registre o CDN no CSP do servidor (ver abaixo). Se o Vite fizer bundle inline do JS, o CSP atual já cobre com `'self'`.

## BACKEND

Servidor Go com `net/http` em `127.0.0.1:<porta>` (default 8080).
Headers injetados pelo servidor:

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-eval' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
```

> Se usar Alpine.js ou Lucide via CDN, avise para ajustarmos o CSP. Se tudo estiver no bundle, o CSP acima já funciona.

## ENDPOINTS

### GET /healthz
```json
{"status":"ok","db":true,"raw_messages":34570,"uptime_seconds":3600.5}
```

### GET /api/channels
```typescript
type ChannelStats = {
  ChannelID: number;
  Username: string;   // pode ser "" → fallback "id:" + ChannelID
  MessageCount: number;
};
```
Retorna array.

### GET /api/messages?limit=100&channel_id=0
```typescript
type RawMessage = {
  ID: number;
  ChannelID: number;
  MessageID: number;
  Payload: string;      // base64 — decodificar com atob() → JSON.parse
  ReceivedAt: string;   // ISO 8601
  SchemaVersion: number;
};
```
Do payload decodificado, extrair preview: `Message` (string) ou `Updates[0].Message.Message`.

### GET /api/message/{id}
Um RawMessage.

### GET /api/processed?limit=100&type=deal_complete
```typescript
type ProcessedMessage = {
  id: number;
  raw_message_id: number;
  channel_id: number;
  message_id: number;
  message_type: string;       // "deal_complete" | "deal_no_coupon" | "deal_no_price" | "coupon_only" | "coupon_expired" | "commentary" | "video" | "admin_meta"
  text_clean: string;
  text_length: number;
  media_type: string;         // "photo" | "video" | "none" | ...
  photo_id: number;           // >0 = tem foto → /api/media/{id}
  has_url: boolean;
  has_price: boolean;
  has_coupon: boolean;
  price_amount: number;       // ⚠️ CENTAVOS! 2990 = R$29,90
  price_currency: string;     // "BRL"
  posted_at: string;          // ISO 8601
  processed_at: string;       // ISO 8601
  price_original: number;     // centavos, 0 se não há
  price_discount: number;     // % desconto 0-100
  coupon_code: string;        // "" se não há
  payment_method: string;     // "pix" | ""
  shipping: string;           // "frete_gratis" | "frete_gratis_prime" | ""
  installments: string;       // "12x_sem_juros" | ""
  shipping_free: boolean;
  installments_n: number;
  installments_value: number; // Reais (já convertido, NÃO centavos)
  discount_percent: number;
  merchant: string;           // "amazon" | "mercadolivre" | "shopee" | ...
  url: string;                // URL do produto, "" se não há
  product_name: string;       // sempre vazio (Fase 3)
  is_duplicate: boolean;      // mesma URL em outro canal
  is_recurring: boolean;      // compra recorrente/assinatura
};
```

### GET /api/processed/stats
```typescript
type ProcessedStats = {
  total: number;
  by_type: { message_type: string; count: number }[];
};
```

### GET /api/media/{id}
- `id` = `processed_messages.id` (não `photo_id`)
- Retorna `image/webp` binário
- HTTP 404 se sem foto
- Header: `Cache-Control: public, max-age=3600`

### GET /api/events (SSE)
EventSource. Eventos: `message`, `stats`. Fallback polling 30s.
Indicador: verde="ao vivo", amarelo="polling 30s", vermelho="off".

## REGRAS DE NEGÓCIO

1. **Preços em centavos.** `price_amount: 2990` = R$ 29,90. Sempre /100.
   `installments_value` já está em Reais (exceção).
2. **Datas ISO 8601.** `new Date(s).toLocaleString('pt-BR')`.
3. **URL:** mostrar o valor do campo `url` como link clicável. Se vazio, mostrar "Não".
   **Jamais** mostrar `has_url` como "Sim"/"Não".
4. **Cupom:** mostrar `coupon_code`. Se vazio, mostrar "Não".
   **Jamais** mostrar `has_coupon` como "Sim"/"Não".
5. **Imagens:** `src="/api/media/{id}"`, lazy loading, remover elemento se 404.
6. **Português brasileiro** para toda interface.
7. **UTF-8.** Acentos: ção, ão, ç, ê, ó.

## FUNCIONALIDADES

- **Tabs**: Raw | Processadas com contagem
- **Stats**: 5 cards (Canais, Raw, Processadas, Health, Uptime)
- **Filtros**: por canal (raw), por tipo (processadas)
- **Busca textual** com debounce 300ms
- **Cards processados**: thumbnail, tipo, canal, data, loja, preço, desconto, cupom, condições
- **Painel detalhe** (overlay/sidebar): todos os campos + texto limpo
- **Conexão SSE** com indicador e fallback polling
- **Estados vazios** amigáveis

## ARQUITETURA SUGERIDA (Alpine + TS)

```
src/
  api.ts              ← fetch wrapper tipado
  stores/
    channels.ts       ← Alpine.store('channels', { ... })
    processed.ts      ← Alpine.store('processed', { ... })
    connection.ts     ← SSE + polling state
  components/
    StatsCards.ts
    RawTab.ts
    ProcessedTab.ts
    DetailPanel.ts
    ConnectionBadge.ts
  types.ts            ← interfaces copiadas deste documento
  main.ts             ← Alpine.data() registrations + init
```

Stores com TypeScript têm autocompletion — use `Alpine.store('channels')` tipado.

## VERIFICAÇÃO

O build final (`dist/`) deve ser servido pelo Go. Os endpoints acima são a única fonte de dados — **não** invente endpoints novos nem assuma campos que não estão documentados aqui.
