import { Agent, Message, NewsroomAPI, Run } from '../types/api';
const baseURL = process.env.EXPO_PUBLIC_API_BASE_URL ?? 'http://localhost:8080';

type ErrorResponse = { error?: { code?: string; message?: string } };

export class APIError extends Error {
  constructor(public readonly status: number, public readonly code?: string, message = 'La requête a échoué.') {
    super(message);
    this.name = 'APIError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    ...init,
    headers: { Accept: 'application/json', 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  });
  const body = (await response.json().catch(() => ({}))) as ErrorResponse & T;
  if (!response.ok) {
    throw new APIError(response.status, body.error?.code, body.error?.message ?? 'La requête a échoué.');
  }
  return body as T;
}

export const httpAPI: NewsroomAPI = {
  async listAgents() { return (await request<{ agents: Agent[] }>('/api/v1/agents')).agents; },
  async listMessages(agentId) { return (await request<{ messages: Message[] }>(`/api/v1/agents/${encodeURIComponent(agentId)}/messages`)).messages; },
  async startRun(agentId) { return (await request<{ run: Run }>(`/api/v1/agents/${encodeURIComponent(agentId)}/runs`, { method: 'POST', body: '{}' })).run; },
  async postMessage() { throw new Error('La révision conversationnelle sera disponible dans une prochaine version.'); },
  async updateDraft() { throw new Error('La modification des brouillons sera disponible dans une prochaine version.'); },
};
