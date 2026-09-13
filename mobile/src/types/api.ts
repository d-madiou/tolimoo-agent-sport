export type RunStatus = 'queued' | 'running' | 'completed' | 'failed';
export type DraftStatus = 'pending' | 'approved' | 'rejected';
export type ClaimStatus = 'official' | 'reported' | 'unverified';
export type MessageRole = 'user' | 'assistant' | 'system';
export type Agent = { id: string; assignment: string; language: string; platforms: string[]; enabled: boolean; researchIntervalSeconds: number; pendingDraftCount: number; isRunning: boolean; createdAt: string; updatedAt: string };
export type Source = { id: string; url: string; title: string; publishedAt: string | null; retrievedAt: string };
export type Draft = { id: string; agentId: string; storyId: string; runId: string; headline: string; claimStatus: ClaimStatus; facebookText: string; xText: string; reviewStatus: DraftStatus; sources: Source[]; createdAt: string; updatedAt: string };
export type Message = { id: string; agentId: string; role: MessageRole; messageType: string; text: string; draftId: string | null; runId: string | null; createdAt: string; draft?: Draft };
export interface NewsroomAPI { listAgents(): Promise<Agent[]>; listMessages(agentId: string): Promise<Message[]>; startRun(agentId: string): Promise<never>; postMessage(agentId: string, text: string): Promise<never>; updateDraft(id: string, update: Partial<Pick<Draft, 'facebookText'|'xText'|'reviewStatus'>>): Promise<never>; }
