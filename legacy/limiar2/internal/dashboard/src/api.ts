import type { ChannelStats, HealthStats, ProcessedMessage, ProcessedStats, RawMessage } from './types';

const BASE_URL = '/api';

export const api = {
  async getHealth(): Promise<HealthStats> {
    const res = await fetch('/healthz');
    if (!res.ok) throw new Error('Health check failed');
    return res.json();
  },
  async getChannels(): Promise<ChannelStats[]> {
    const res = await fetch(`${BASE_URL}/channels`);
    if (!res.ok) throw new Error('Failed to fetch channels');
    return res.json();
  },
  async getRawMessages(limit: number = 100, channelId?: number): Promise<RawMessage[]> {
    let url = `${BASE_URL}/messages?limit=${limit}`;
    if (channelId !== undefined) url += `&channel_id=${channelId}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error('Failed to fetch raw messages');
    return res.json();
  },
  async getRawMessage(id: number): Promise<RawMessage> {
    const res = await fetch(`${BASE_URL}/message/${id}`);
    if (!res.ok) throw new Error('Failed to fetch raw message');
    return res.json();
  },
  async getProcessedMessages(limit: number = 100, type?: string, merchant?: string): Promise<ProcessedMessage[]> {
    let url = `${BASE_URL}/processed?limit=${limit}`;
    if (type) url += `&type=${encodeURIComponent(type)}`;
    if (merchant) url += `&merchant=${encodeURIComponent(merchant)}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error('Failed to fetch processed messages');
    return res.json();
  },
  async getProcessedStats(): Promise<ProcessedStats> {
    const res = await fetch(`${BASE_URL}/processed/stats`);
    if (!res.ok) throw new Error('Failed to fetch processed stats');
    return res.json();
  },
  getMediaUrl(id: number): string {
    return `${BASE_URL}/media/${id}`;
  }
};
