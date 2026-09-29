import Alpine from 'alpinejs';
import { createIcons, icons } from 'lucide';
import { api } from './api';
import type { ChannelStats, HealthStats, ProcessedMessage, RawMessage, ProcessedStats } from './types';
import './index.css';

declare global {
  interface Window {
    Alpine: any;
  }
}

// Disponibiliza o Alpine globalmente.
window.Alpine = Alpine;

const formatPrice = (cents: number) => {
  return (cents / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' });
};
const formatDate = (isoStr: string) => {
  try {
    return new Date(isoStr).toLocaleString('pt-BR');
  } catch (e) {
    return isoStr;
  }
};
const decodePayloadPreview = (payload: string) => {
  try {
    const json = JSON.parse(atob(payload));
    return json.Message || (json.Updates && json.Updates[0]?.Message?.Message) || 'Prévia indisponível';
  } catch (e) {
    return 'Payload inválido';
  }
};
const getBadgeColor = (type: string) => {
  switch (type) {
    case 'deal_complete': return 'bg-green-500/10 text-green-500 border-green-500/20';
    case 'deal_no_coupon': return 'bg-blue-500/10 text-blue-500 border-blue-500/20';
    case 'deal_no_price': return 'bg-amber-500/10 text-amber-500 border-amber-500/20';
    case 'coupon_only': return 'bg-indigo-500/10 text-indigo-500 border-indigo-500/20';
    case 'coupon_expired': return 'bg-red-900/30 text-red-400 border-red-900/50';
    case 'commentary': return 'bg-neutral-500/10 text-neutral-400 border-neutral-500/20';
    default: return 'bg-neutral-500/10 text-neutral-400 border-neutral-500/20';
  }
};

document.addEventListener('alpine:init', () => {

  Alpine.store('global', {
    health: null as HealthStats | null,
    channels: [] as ChannelStats[],
    processedStats: null as ProcessedStats | null,
    connectionStatus: 'connecting' as 'connected' | 'polling' | 'disconnected',
    detailMessage: null as any,
    detailType: 'raw' as 'raw' | 'processed',
    isDetailOpen: false,

    async init() {
      await this.fetchInitialData();
      this.setupSSE();
    },

    async fetchInitialData() {
      try {
        const [health, channels, stats] = await Promise.all([
          api.getHealth(),
          api.getChannels(),
          api.getProcessedStats()
        ]);
        this.health = health;
        this.channels = channels;
        this.processedStats = stats;
      } catch (err) {
        console.error('Erro na carga inicial', err);
      }
    },

    setupSSE() {
      try {
        const evSource = new EventSource('/api/events');
        evSource.onopen = () => {
          this.connectionStatus = 'connected';
        };
        evSource.onmessage = (event) => {
          // Dispara uma atualização genérica.
          this.fetchInitialData(); // Atualização simples das estatísticas.
        };
        evSource.onerror = (err) => {
          this.connectionStatus = 'polling';
          evSource.close();
          this.fallbackPolling();
        };
      } catch (e) {
        this.connectionStatus = 'polling';
        this.fallbackPolling();
      }
    },

    fallbackPolling() {
      setInterval(() => {
        this.fetchInitialData();
      }, 30000);
    },

    openDetail(item: any, type: 'raw' | 'processed') {
      this.detailMessage = item;
      this.detailType = type;
      this.isDetailOpen = true;
    },

    closeDetail() {
      this.isDetailOpen = false;
      this.detailMessage = null;
    }
  });

  Alpine.data('dashboard', () => ({
    activeTab: 'processed',
  }));

  Alpine.data('rawTab', () => ({
    messages: [] as RawMessage[],
    loading: true,
    error: null as string | null,
    search: '',
    channelId: '' as string | number,

    init() {
      this.fetchMessages();
      this.$watch('search', () => this.debouncedFetch());
      this.$watch('channelId', () => this.fetchMessages());
    },

    debouncedFetch: (() => {
      let timeout: any;
      return function(this: any) {
        clearTimeout(timeout);
        timeout = setTimeout(() => this.fetchMessages(), 300);
      };
    })(),

    async fetchMessages() {
      this.loading = true;
      this.error = null;
      try {
        const cid = this.channelId ? Number(this.channelId) : undefined;
        let data = await api.getRawMessages(100, cid);
        if (this.search.trim()) {
           const query = this.search.toLowerCase();
           data = data.filter(m => decodePayloadPreview(m.Payload).toLowerCase().includes(query));
        }
        this.messages = data;
      } catch (e) {
        this.error = 'Erro ao carregar mensagens brutas';
      } finally {
        this.loading = false;
      }
    },
    
    getPreview(payload: string) {
       return decodePayloadPreview(payload);
    },
    formatDate
  }));

  Alpine.data('processedTab', () => ({
    messages: [] as ProcessedMessage[],
    loading: true,
    error: null as string | null,
    search: '',
    typeFilter: '',
    merchantFilter: '',

    init() {
      this.fetchMessages();
      this.$watch('search', () => this.debouncedFetch());
      this.$watch('typeFilter', () => this.fetchMessages());
      this.$watch('merchantFilter', () => this.fetchMessages());
    },

    debouncedFetch: (() => {
      let timeout: any;
      return function(this: any) {
        clearTimeout(timeout);
        timeout = setTimeout(() => this.fetchMessages(), 300);
      };
    })(),

    async fetchMessages() {
      this.loading = true;
      this.error = null;
      try {
        let data = await api.getProcessedMessages(100, this.typeFilter || undefined, this.merchantFilter || undefined);
        if (this.search.trim()) {
           const query = this.search.toLowerCase();
           data = data.filter(m => m.text_clean.toLowerCase().includes(query));
        }
        this.messages = data;
      } catch (e) {
        this.error = 'Erro ao carregar mensagens processadas';
      } finally {
        this.loading = false;
      }
    },
    formatPrice,
    formatDate,
    getBadgeColor,
    getMediaUrl: api.getMediaUrl
  }));

  Alpine.data('detailPanel', () => ({
    get detailMessage() { return Alpine.store('global').detailMessage; },
    get detailType() { return Alpine.store('global').detailType; },
    get isOpen() { return Alpine.store('global').isDetailOpen; },
    
    close() { Alpine.store('global').closeDetail(); },
    
    get rawPayload() {
       if (this.detailType !== 'raw' || !this.detailMessage) return '';
       try {
         return JSON.stringify(JSON.parse(atob(this.detailMessage.Payload)), null, 2);
       } catch (e) {
         return 'Payload Base64/JSON inválido';
       }
    },

    copyCoupon() {
       if(this.detailMessage?.coupon_code) {
          navigator.clipboard.writeText(this.detailMessage.coupon_code);
       }
    },
    formatPrice,
    formatDate,
    getBadgeColor,
    getMediaUrl: api.getMediaUrl
  }));

});

Alpine.start();

// Inicializa os ícones.
document.addEventListener('DOMContentLoaded', () => {
  // O Lucide exige createIcons para substituir as tags i-lucide,
  // mas o Alpine cria elementos do DOM dinamicamente; por isso,
  // precisamos de um observador de mutações ou helper equivalente.
  const observer = new MutationObserver(() => {
    createIcons({ icons, nameAttr: 'data-lucide' });
  });
  observer.observe(document.body, { childList: true, subtree: true });
  createIcons({ icons, nameAttr: 'data-lucide' });
});

