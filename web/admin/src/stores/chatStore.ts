import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import type { ChatMessage, ContextListItem, ToolCall, ThinkingBlock } from '../types/chat';

interface ChatState {
  // Current chat
  agentName: string | null;
  contextId: string | null;
  messages: ChatMessage[];
  isStreaming: boolean;
  error: string | null;

  // Context list
  contexts: ContextListItem[];

  // Actions
  setAgentName: (name: string) => void;
  setContextId: (id: string | null) => void;
  setMessages: (messages: ChatMessage[]) => void;
  addMessage: (message: ChatMessage) => void;
  updateMessage: (taskId: string, updates: Partial<ChatMessage>) => void;
  setStreaming: (isStreaming: boolean) => void;
  setError: (error: string | null) => void;
  setContexts: (contexts: ContextListItem[]) => void;
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

    appendToLastMessage: (content, field) => set((state) => {
      if (state.messages.length === 0) return state;
      const lastIdx = state.messages.length - 1;
      const messages = [...state.messages];
      const currentContent = messages[lastIdx][field] || '';
      messages[lastIdx] = {
        ...messages[lastIdx],
        [field]: currentContent + content,
      };
      return { messages };
    }),

    addToolCall: (taskId, toolCall) => set((state) => {
      const messages = state.messages.map((m) => {
        if (m.task_id === taskId) {
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
        if (m.task_id === taskId && m.tool_calls) {
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
        if (m.task_id === taskId) {
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
    }),
  }))
);