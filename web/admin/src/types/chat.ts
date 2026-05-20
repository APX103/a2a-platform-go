// Message roles
export type MessageRole = 'user' | 'assistant' | 'system' | 'tool';

// Message types
export interface ChatMessage {
  id?: number;
  task_id?: string;
  context_id?: string;
  role: MessageRole;
  content: string;
  reasoning_content?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  thinking_blocks?: ThinkingBlock[];
  timestamp?: string;
}

// Tool call representation
export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
  arguments_obj?: Record<string, unknown>;
  result?: string;
  status: 'started' | 'completed' | 'error';
  start_time?: string;
  end_time?: string;
  metadata?: Record<string, unknown>;
}

// Thinking block
export interface ThinkingBlock {
  id: string;
  timestamp: number;
  content: string;
  duration_ms?: number;
}

// Context/Session types
export interface Context {
  id: string;
  agent_name: string;
  title: string;
  message_count: number;
  created_at: string;
  updated_at: string;
}

export interface ContextListItem {
  id: string;
  agent_name: string;
  title: string;
  message_count: number;
  created_at: string;
  updated_at: string;
}

export interface ContextDetail {
  context: Context;
  messages: ChatMessage[];
}

// SSE event types
export type SSEEventType =
  | 'text.delta'
  | 'thinking.delta'
  | 'thinking.block'
  | 'tool.call_start'
  | 'tool.call_delta'
  | 'tool.call_end'
  | 'tool.result'
  | 'task.status'
  | 'subagent.started'
  | 'subagent.completed'
  | 'subagent.error'
  | 'error'
  | 'done';

export interface SSEEvent {
  type: SSEEventType;
  task_id?: string;
  context_id?: string;
  text?: string;
  thinking?: string;
  tool?: ToolCall;
  error?: string;
  status?: { state: string; message?: unknown };
  subagent_id?: string;
  subagent_task?: string;
  metadata?: Record<string, unknown>;
}

// Subagent types
export interface SubagentSession {
  id: string;
  parent_context_id: string;
  parent_tool_call_id: string;
  task: string;
  context: string;
  status: 'running' | 'completed' | 'failed' | 'timeout';
  messages?: string;
  result?: string;
  error?: string;
  created_at: string;
  completed_at?: string;
}

// API responses
export interface ListContextsResponse {
  items: ContextListItem[];
  total: number;
  page: number;
  size: number;
}

export interface CreateContextRequest {
  agent_name: string;
  title?: string;
}

export interface UpdateContextTitleRequest {
  title: string;
}