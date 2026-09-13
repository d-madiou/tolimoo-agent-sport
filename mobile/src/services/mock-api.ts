import { NewsroomAPI, Run } from '../types/api';
import { demoAgents, demoMessages } from '../fixtures/newsroom';

let mockRunNumber = 0;

export const mockAPI: NewsroomAPI = {
  async listAgents() { return demoAgents; },
  async listMessages(agentId) { return demoMessages[agentId] ?? []; },
  async startRun(agentId) {
    mockRunNumber += 1;
    return { id: `demo-run-${mockRunNumber}`, agentId, status: 'queued', startedAt: new Date().toISOString(), endedAt: null, error: null } satisfies Run;
  },
  async postMessage() { throw new Error('Demo mode: messages are not persisted.'); },
  async updateDraft() { throw new Error('Demo mode: draft changes are not persisted.'); },
};
export const isMockMode = process.env.EXPO_PUBLIC_USE_MOCK_API === 'true';
