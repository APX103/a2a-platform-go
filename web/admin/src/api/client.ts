const BASE = '';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
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

import type {
  Context,
  ContextListItem,
  ContextDetail,
  ListContextsResponse,
  CreateContextRequest,
  TaskSession,
} from '../types/chat';

export interface Agent {
  name: string;
  url: string;
  description?: string;
  skills?: string[] | null;
  status?: string;
  type?: string;
  version?: string;
  registered_at?: string;
  last_seen?: string;
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

export const api = {
  getHealth: () => request<HealthResponse>('/health'),

  listAgents: () => request<Agent[]>('/api/agents'),
  getAgent: (name: string) => request<Agent>(`/api/agents/${name}`),
  registerAgent: (agent: Partial<Agent>, token: string) =>
    request<Agent>('/api/agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Admin-Token': token },
      body: JSON.stringify(agent),
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
};
