import { NewsroomAPI } from '../types/api'; import { demoAgents, demoMessages } from '../fixtures/newsroom';
export const mockAPI: NewsroomAPI = { async listAgents(){ return demoAgents; }, async listMessages(agentId){ return demoMessages[agentId] ?? []; }, async startRun(){ throw new Error('Demo mode: research is not persisted.'); }, async postMessage(){ throw new Error('Demo mode: messages are not persisted.'); }, async updateDraft(){ throw new Error('Demo mode: draft changes are not persisted.'); } };
export const isMockMode = process.env.EXPO_PUBLIC_USE_MOCK_API === 'true';
