import { safeStorage } from '../utils/storage';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL?.trim() ?? '';
const BASE = API_BASE_URL.endsWith('/') ? API_BASE_URL.slice(0, -1) : API_BASE_URL;
const DEV_ADMIN_TOKEN = import.meta.env.DEV ? import.meta.env.VITE_DEV_ADMIN_TOKEN?.trim() ?? '' : '';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = safeStorage.getItem('admin_token') || DEV_ADMIN_TOKEN;
  const optionHeaders = headersToObject(options?.headers);
  const hasExplicitAuth = Object.keys(optionHeaders).some(key => {
    const normalized = key.toLowerCase();
    return normalized === 'authorization' || normalized === 'x-admin-token' || normalized === 'x-group-member-token';
  });
  const headers = {
    'Content-Type': 'application/json',
    ...(token && !hasExplicitAuth ? { 'X-Admin-Token': token } : {}),
    ...optionHeaders,
  };
  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  // Handle 204 No Content or empty body
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

async function streamRequest(path: string, options: RequestInit, onEvent: (event: GroupStreamEvent) => void): Promise<void> {
  const token = safeStorage.getItem('admin_token') || DEV_ADMIN_TOKEN;
  const optionHeaders = headersToObject(options.headers);
  const hasExplicitAuth = Object.keys(optionHeaders).some(key => {
    const normalized = key.toLowerCase();
    return normalized === 'authorization' || normalized === 'x-admin-token' || normalized === 'x-group-member-token';
  });
  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token && !hasExplicitAuth ? { 'X-Admin-Token': token } : {}),
      ...optionHeaders,
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  if (!res.body) return;

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf('\n\n');
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const data = frame
        .split('\n')
        .map(line => line.trim())
        .filter(line => line.startsWith('data:'))
        .map(line => line.slice(5).trim())
        .join('\n');
      if (data) {
        onEvent(JSON.parse(data) as GroupStreamEvent);
      }
      boundary = buffer.indexOf('\n\n');
    }
  }
}

function headersToObject(headers?: HeadersInit): Record<string, string> {
  if (!headers) return {};
  if (headers instanceof Headers) {
    const result: Record<string, string> = {};
    headers.forEach((value, key) => { result[key] = value; });
    return result;
  }
  if (Array.isArray(headers)) {
    return Object.fromEntries(headers.map(([key, value]) => [key, value]));
  }
  return { ...headers };
}

import type {
  Context,
  ContextDetail,
  ListContextsResponse,
  CreateContextRequest,
  TaskSession,
} from '../types/chat';

export interface Agent {
  name: string;
  url: string;
  description?: string;
  skills?: Array<string | { id?: string; name?: string; description?: string }> | null;
  status?: string;
  type?: string;
  version?: string;
  context_mode?: string;
  agent_card_json?: string;
  simple_mode?: boolean;
  registered_at?: string;
  last_seen?: string;
}

export interface AgentCard {
  name?: string;
  description?: string;
  version?: string;
  url?: string;
  skills?: Array<{ id?: string; name?: string; description?: string; tags?: string[]; examples?: string[] }>;
  health_url?: string;
  x_static?: boolean;
  x_context_mode?: string;
}

export interface Task {
  local_task_id: string;
  display_id?: string;
  server_task_id?: string | null;
  source_agent?: string | null;
  target_agent?: string;
  agent_name: string;
  state: string;
  context_id?: string;
  root_context_id?: string | null;
  parent_task_id?: string | null;
  parent_tool_call_id?: string | null;
  created_at: string;
  updated_at?: string;
}

export interface Message {
  id?: number;
  task_id?: string;
  role: string;
  sender_agent?: string | null;
  recipient_agent?: string | null;
  content: string;
  reasoning_content?: string;
  tool_calls?: string;
  thinking_blocks?: string;
  timestamp?: string;
}

export interface Trace {
  id?: number;
  task_id?: string;
  context_id?: string;
  root_context_id?: string | null;
  parent_task_id?: string | null;
  agent_name?: string;
  target_agent?: string;
  event_type?: string;
  data_json?: string;
  timestamp?: string;
  duration_ms?: number | null;
}

export interface TaskDetail {
  task: Task;
  messages: Message[];
  traces: Trace[];
}

export interface TaskListResponse {
  items: Task[];
  total?: number;
  page?: number;
  size?: number;
}

export interface TraceContextSummary {
  context_id: string;
  trace_count: number;
  last_active: string;
  agents?: string[];
}

export interface HealthResponse {
  status: string;
  db?: string;
  agents_connected?: number;
  agents_total?: number;
}

export interface BuiltinAgent {
  name: string;
  provider: string;
  base_url: string;
  model: string;
  description: string;
  system_prompt: string;
  max_tokens: number;
  max_tool_rounds: number;
}

export interface HumanPresence {
  id: string;
  handle: string;
  display_name: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
  active_sessions: number;
  online: boolean;
  status: 'online' | 'offline' | string;
}

export interface HumanTokenIssueResponse {
  human: { id: string; handle: string; display_name: string };
  session_token: string;
  expires_at?: string;
}

export interface HumanProfile {
  id: string;
  handle: string;
  display_name: string;
}

export interface AgentCredentialResponse {
  name: string;
  secret: string;
  available: boolean;
}

export interface CreateBuiltinAgentReq {
  name: string;
  provider: string;
  base_url: string;
  api_key: string;
  model: string;
  description?: string;
  system_prompt?: string;
  max_tokens?: number;
  max_tool_rounds?: number;
}

export interface Group {
  id: string;
  name: string;
  description?: string;
  orchestration_mode: 'p2p' | 'leader_led' | 'free_chat' | 'roundtable' | 'stateflow' | 'research_long_horizon' | string;
  rules_json?: string;
  memory_policy_json?: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface CreateGroupReq {
  name: string;
  description?: string;
  orchestration_mode?: string;
  rules?: unknown;
  memory_policy?: unknown;
}

export interface GroupMember {
  id: number;
  group_id: string;
  actor_type: 'agent' | 'human' | 'system' | string;
  actor_id: string;
  role: string;
  capabilities_json?: string;
  joined_at: string;
}

export interface GroupEvent {
  id: number;
  group_id: string;
  event_type: string;
  sender_type: 'agent' | 'human' | 'system' | string;
  sender_id: string;
  content: string;
  metadata_json?: string;
  created_at: string;
}

export interface GroupArtifact {
  id: string;
  group_id: string;
  name: string;
  artifact_type: string;
  version: number;
  content: string;
  status: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface GroupOrchestrationState {
  group_id: string;
  mode: string;
  next_action: string;
  eligible_speakers: string[];
  context_policy: string;
  termination_policy: string;
}

export interface GroupInvite {
  id: number;
  group_id: string;
  actor_type_allowed?: string;
  role: string;
  max_uses: number;
  used_count: number;
  expires_at?: string;
  status: string;
  created_at: string;
  token?: string;
}

export interface GroupEventResponse {
  event: GroupEvent;
  orchestration: GroupOrchestrationState;
  triggered?: GroupEvent[];
}

export type GroupStreamEvent =
  | { type: 'group.event'; event: GroupEvent }
  | { type: 'group.agent_start'; sender_id: string; sender_type: string }
  | { type: 'group.agent_delta'; sender_id: string; sender_type: string; text: string }
  | { type: 'group.agent_thinking'; sender_id: string; sender_type: string; thinking: string }
  | { type: 'group.agent_skip'; sender_id: string; sender_type: string }
  | { type: 'group.artifact'; artifact: GroupArtifact }
  | { type: 'group.done'; event: GroupEvent; orchestration: GroupOrchestrationState; triggered?: GroupEvent[] }
  | { type: 'group.error'; error: string };

export interface GroupJoinResponse {
  group: Group;
  member: GroupMember;
  access_token: string;
  orchestration: GroupOrchestrationState;
}

export const api = {
  getHealth: () => request<HealthResponse>('/health'),
  validateAdminToken: async (token: string) => {
    const res = await fetch(`${BASE}/api/agents`, {
      headers: { 'X-Admin-Token': token },
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`${res.status}: ${text}`);
    }
  },

  listAgents: () => request<Agent[]>('/api/agents'),
  listHumans: () => request<HumanPresence[]>('/api/humans'),
  getHuman: (id: string) => request<HumanProfile>(`/api/humans/${encodeURIComponent(id)}`),
  updateHuman: (id: string, human: { handle?: string; display_name?: string }) =>
    request<HumanProfile>(`/api/humans/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(human),
    }),
  issueHumanToken: (id: string) =>
    request<HumanTokenIssueResponse>(`/api/humans/${encodeURIComponent(id)}/tokens`, {
      method: 'POST',
    }),
  deleteHuman: (id: string) =>
    request<void>(`/api/humans/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  getAgent: (name: string) => request<Agent>(`/api/agents/${name}`),
  getAgentCredential: (name: string) => request<AgentCredentialResponse>(`/api/agents/${encodeURIComponent(name)}/credential`),
  registerAgent: (agent: Partial<Agent>, token: string) =>
    request<Agent>('/api/agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(agent),
    }),
  updateAgent: (name: string, req: { url?: string; port?: number; context_mode?: string; agent_card?: AgentCard }, token: string) =>
    request<Agent>(`/api/agents/${name}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(req),
    }),
  deleteAgent: (name: string, token: string) =>
    request<void>(`/api/agents/${name}`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
    }),

  listTasks: (params?: { agent_name?: string; state?: string; search?: string; context_id?: string; page?: number; size?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.agent_name) searchParams.set('agent_name', params.agent_name);
    if (params?.state) searchParams.set('state', params.state);
    if (params?.search) searchParams.set('search', params.search);
    if (params && Object.prototype.hasOwnProperty.call(params, 'context_id')) searchParams.set('context_id', params.context_id ?? '');
    if (params?.page) searchParams.set('page', String(params.page));
    if (params?.size) searchParams.set('size', String(params.size));
    const qs = searchParams.toString();
    return request<TaskListResponse>(`/api/tasks${qs ? '?' + qs : ''}`);
  },
  getTask: (id: string) => request<TaskDetail>(`/api/tasks/${id}`),
  listTasksByRoot: (rootContextId: string) => request<Task[]>(`/api/tasks/root/${rootContextId}`),
  listRecentTraces: () => request<Trace[]>('/api/traces'),
  listTraceContexts: () => request<TraceContextSummary[]>('/api/traces/contexts'),
  listTracesByContext: (contextId: string) => request<Trace[]>(`/api/traces/context/${contextId}`),
  listTracesByRoot: (rootContextId: string) => request<Trace[]>(`/api/traces/root/${rootContextId}`),

  listBuiltinAgents: () => request<BuiltinAgent[]>('/api/builtin-agents'),
  createBuiltinAgent: (agent: CreateBuiltinAgentReq, token: string) =>
    request<{ ok: boolean }>('/api/builtin-agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(agent),
    }),
  updateBuiltinAgent: (name: string, agent: CreateBuiltinAgentReq, token: string) =>
    request<{ ok: boolean }>(`/api/builtin-agents/${name}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(agent),
    }),
  deleteBuiltinAgent: (name: string, token: string) =>
    request<void>(`/api/builtin-agents/${name}`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
    }),

  // Context API
  listContexts: (agentName: string, params?: { page?: number; size?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.page) searchParams.set('page', String(params.page));
    if (params?.size) searchParams.set('size', String(params.size));
    const qs = searchParams.toString();
    return request<ListContextsResponse>(`/api/contexts/${agentName}${qs ? '?' + qs : ''}`);
  },

  getContext: (id: string) => request<ContextDetail>(`/api/contexts/${id}`),

  createContext: (req: CreateContextRequest) => request<Context>('/api/contexts/', {
    method: 'POST',
    body: JSON.stringify(req),
  }),

  deleteContext: (id: string, token?: string) => request<void>(`/api/contexts/${id}`, {
    method: 'DELETE',
    ...(token && { headers: { 'X-Admin-Token': token } }),
  }),

  updateContextTitle: (id: string, title: string) => request<Context>(`/api/contexts/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ title }),
  }),

  // Subagent API
  listSubagents: (contextId: string) => request<{ context_id: string; subagents: TaskSession[] }>(`/api/subagents/${contextId}`),

  getSubagent: (id: string) => request<TaskSession>(`/api/subagents/${id}`),

  // Group orchestration API
  listGroups: (status?: string) => request<Group[]>(`/api/groups${status ? `?status=${encodeURIComponent(status)}` : ''}`),
  createGroup: (group: CreateGroupReq, token: string) =>
    request<Group>('/api/groups', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(group),
    }),
  getGroup: (id: string) => request<Group>(`/api/groups/${id}`),
  updateGroup: (id: string, group: Partial<CreateGroupReq> & { status?: string }, token: string) =>
    request<Group>(`/api/groups/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(group),
    }),
  archiveGroup: (id: string, token: string) =>
    request<void>(`/api/groups/${id}`, {
      method: 'DELETE',
      headers: { 'X-Admin-Token': token },
    }),
  listGroupMembers: (id: string) => request<GroupMember[]>(`/api/groups/${id}/members`),
  listGroupInvites: (id: string) => request<GroupInvite[]>(`/api/groups/${id}/invites`),
  createGroupInvite: (id: string, invite: { actor_type_allowed?: string; role?: string; max_uses?: number; expires_at?: string }, token: string) =>
    request<GroupInvite>(`/api/groups/${id}/invites`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(invite),
    }),
  addGroupMember: (id: string, member: { actor_type: string; actor_id: string; role?: string; capabilities?: unknown }, token: string) =>
    request<GroupMember[]>(`/api/groups/${id}/members`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(member),
    }),
  removeGroupMember: (id: string, actorType: string, actorId: string, token: string) =>
    request<GroupMember[]>(`/api/groups/${id}/members/${encodeURIComponent(actorType)}/${encodeURIComponent(actorId)}`, {
      method: 'DELETE',
      headers: { 'X-Admin-Token': token },
    }),
  joinGroup: (id: string, client: { client_id: string; capabilities?: unknown }, token?: string) =>
    request<GroupMember>(`/api/groups/${id}/join`, {
      method: 'POST',
      ...(token ? { headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token } } : {}),
      body: JSON.stringify(client),
    }),
  joinGroupByInvite: (req: { invite_token: string; actor_type?: string; actor_id: string; client_id?: string; capabilities?: unknown }) =>
    request<GroupJoinResponse>('/api/group-joins', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  listGroupEvents: (id: string, limit?: number) =>
    request<GroupEvent[]>(`/api/groups/${id}/events${limit ? `?limit=${limit}` : ''}`),
  appendGroupEvent: (id: string, event: { event_type?: string; sender_type: string; sender_id: string; content: string; metadata?: unknown }, memberToken?: string) =>
    request<GroupEventResponse>(`/api/groups/${id}/events`, {
      method: 'POST',
      ...(memberToken ? { headers: { Authorization: `Bearer ${memberToken}` } } : {}),
      body: JSON.stringify(event),
    }),
  streamGroupEvent: (
    id: string,
    event: { event_type?: string; sender_type: string; sender_id: string; content: string; metadata?: unknown },
    handlers: { onEvent: (event: GroupStreamEvent) => void },
    memberToken?: string,
  ) => streamRequest(`/api/groups/${id}/events`, {
    method: 'POST',
    headers: {
      Accept: 'text/event-stream',
      ...(memberToken ? { Authorization: `Bearer ${memberToken}` } : {}),
    },
    body: JSON.stringify(event),
  }, handlers.onEvent),
  listGroupArtifacts: (id: string) => request<GroupArtifact[]>(`/api/groups/${id}/artifacts`),
  createGroupArtifact: (id: string, artifact: { name: string; artifact_type?: string; content: string; status?: string; created_by?: string }, memberToken?: string) =>
    request<GroupArtifact>(`/api/groups/${id}/artifacts`, {
      method: 'POST',
      ...(memberToken ? { headers: { Authorization: `Bearer ${memberToken}` } } : {}),
      body: JSON.stringify(artifact),
    }),
  updateGroupArtifact: (groupId: string, artifactId: string, artifact: Partial<GroupArtifact>, token: string) =>
    request<GroupArtifact>(`/api/groups/${groupId}/artifacts/${artifactId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(artifact),
    }),
  getGroupOrchestration: (id: string) => request<GroupOrchestrationState>(`/api/groups/${id}/orchestration`),
};
