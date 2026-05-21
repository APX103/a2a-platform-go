import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import type { ChatMessage, ContextListItem, ToolCall, ThinkingBlock, TaskSession } from '../types/chat';

interface ChatState {
  // Current chat
  agentName: string | null;
  contextId: string | null;
  messages: ChatMessage[];
  isStreaming: boolean;
  error: string | null;

  // Context list
  contexts: ContextListItem[];

  // Subagents keyed by tool_call_id
  subagents: Record<string, TaskSession>;

  // Actions
  setAgentName: (name: string) => void;
  setContextId: (id: string | null) => void;
  setMessages: (messages: ChatMessage[]) => void;
  addMessage: (message: ChatMessage) => void;
  updateMessage: (taskId: string, updates: Partial<ChatMessage>) => void;
  setStreaming: (isStreaming: boolean) => void;
  setError: (error: string | null) => void;
  setContexts: (contexts: ContextListItem[]) => void;
  setSubagent: (toolCallId: string, subagent: TaskSession) => void;
  updateSubagent: (toolCallId: string, updates: Partial<TaskSession>) => void;
  setSubagents: (subagents: Record<string, TaskSession>) => void;
  appendToLastMessage: (content: string, field: 'content' | 'reasoning_content') => void;
  addToolCall: (taskId: string, toolCall: ToolCall) => void;
  updateToolCall: (taskId: string, toolId: string, updates: Partial<ToolCall>) => void;
  addThinkingBlock: (taskId: string, block: ThinkingBlock) => void;
  clearChat: () => void;
}

export const useChatStore = create<ChatState>()(
  subscribeWithSelector((set, get) => ({
    // Initial state
    agentName: null,
    contextId: null,
    messages: [],
    isStreaming: false,
    error: null,
    contexts: [],
    subagents: {},

    // Actions
    setAgentName: (name) => set({ agentName: name }),

    setContextId: (id) => set({ contextId: id }),

    setMessages: (messages) => set({ messages }),

    addMessage: (message) => set((state) => ({
      messages: [...state.messages, message],
    })),

    updateMessage: (taskId, updates) => set((state) => {
      const messages = state.messages.map((m) =>
        m.task_id === taskId ? { ...m, ...updates } : m
      );
      return { messages };
    }),

    setStreaming: (isStreaming) => set({ isStreaming }),

    setError: (error) => set({ error }),

    setContexts: (contexts) => set({ contexts }),

    setSubagent: (toolCallId, subagent) => set((state) => ({
      subagents: { ...state.subagents, [toolCallId]: subagent },
    })),

    setSubagents: (subagents) => set({ subagents }),

    updateSubagent: (toolCallId, updates) => set((state) => {
      const existing = state.subagents[toolCallId];
      if (!existing) return state;
      return {
        subagents: {
          ...state.subagents,
          [toolCallId]: { ...existing, ...updates },
        },
      };
    }),

    appendToLastMessage: (content, field) => set((state) => {
      if (state.messages.length === 0) return state;
      // Find the last assistant message to append streaming content
      const lastAssistantIdx = state.messages.map((m) => m.role).lastIndexOf('assistant');
      if (lastAssistantIdx === -1) return state;
      const messages = [...state.messages];
      const currentContent = messages[lastAssistantIdx][field] || '';
      messages[lastAssistantIdx] = {
        ...messages[lastAssistantIdx],
        [field]: currentContent + content,
      };
      return { messages };
    }),

    addToolCall: (taskId, toolCall) => set((state) => {
      const messages = state.messages.map((m) => {
        if (m.task_id === taskId && m.role === 'assistant') {
          const toolCalls = m.tool_calls || [];
          return {
            ...m,
            tool_calls: [...toolCalls, toolCall],
          };
        }
        return m;
      });
      return { messages };
    }),

    updateToolCall: (taskId, toolId, updates) => set((state) => {
      const messages = state.messages.map((m) => {
        if (m.task_id === taskId && m.role === 'assistant' && m.tool_calls) {
          const toolCalls = m.tool_calls.map((tc) =>
            tc.id === toolId ? { ...tc, ...updates } : tc
          );
          return { ...m, tool_calls: toolCalls };
        }
        return m;
      });
      return { messages };
    }),

    addThinkingBlock: (taskId, block) => set((state) => {
      const messages = state.messages.map((m) => {
        if (m.task_id === taskId && m.role === 'assistant') {
          const blocks = m.thinking_blocks || [];
          return {
            ...m,
            thinking_blocks: [...blocks, block],
          };
        }
        return m;
      });
      return { messages };
    }),

    clearChat: () => set({
      messages: [],
      isStreaming: false,
      error: null,
      subagents: {},
    }),
  }))
);