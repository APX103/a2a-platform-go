import { useCallback, useRef, useState } from 'react';
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { useChatStore } from '../stores/chatStore';
import { api } from '../api/client';
import type { SSEEvent, ToolCall, ThinkingBlock, TaskSession } from '../types/chat';

export function useChat(agentName: string) {
  const {
    setAgentName,
    setContextId,
    setMessages,
    addMessage,
    updateMessage,
    setStreaming,
    setError,
    addToolCall,
    updateToolCall,
    addThinkingBlock,
    setSubagent,
    updateSubagent,
    setSubagents,
    appendToLastMessage,
  } = useChatStore();

  const controllerRef = useRef<AbortController | null>(null);
  const [currentTaskId, setCurrentTaskId] = useState<string | null>(null);
  const toolCallBufferRef = useRef<Record<string, ToolCall>>({});
  const [thinkingBuffer, setThinkingBuffer] = useState<{ [taskId: string]: string }>({});

  // Clean up SSE connection
  const disconnect = useCallback(() => {
    if (controllerRef.current) {
      controllerRef.current.abort();
      controllerRef.current = null;
    }
    setStreaming(false);
  }, [setStreaming]);

  // Send message to agent
  const sendMessage = useCallback(
    async (content: string, contextId?: string) => {
      disconnect();

      const controller = new AbortController();
      controllerRef.current = controller;

      setAgentName(agentName);
      setStreaming(true);
      setError(null);

      // Add user message
      const taskId = `task-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
      setCurrentTaskId(taskId);

      addMessage({
        role: 'user',
        content,
        task_id: taskId,
        context_id: contextId || undefined,
        timestamp: new Date().toISOString(),
      });

      // Add empty assistant message to receive streaming content
      addMessage({
        role: 'assistant',
        content: '',
        task_id: taskId,
        context_id: contextId || undefined,
        timestamp: new Date().toISOString(),
      });

      // Prepare request body
      const requestBody = {
        jsonrpc: '2.0',
        id: '1',
        method: 'SendStreamingMessage',
        params: {
          message: {
            role: 'ROLE_USER',
            parts: [{ text: content }],
          },
          ...(contextId && { contextId }),
        },
      };

      try {
        await fetchEventSource(`/agent/${agentName}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(requestBody),
          signal: controller.signal,
          onmessage: (event) => {
            const data: SSEEvent = JSON.parse(event.data);

            switch (data.type) {
              case 'text.delta':
                // Stream text content to last assistant message
                appendToLastMessage(data.text || '', 'content');
                break;

              case 'thinking.delta':
                // Stream thinking content
                if (data.thinking) {
                  const tid = data.task_id || taskId;
                  setThinkingBuffer((prev) => ({
                    ...prev,
                    [tid]: (prev[tid] || '') + data.thinking,
                  }));
                  // Also append to the last assistant message's reasoning_content for display
                  appendToLastMessage(data.thinking, 'reasoning_content');
                }
                break;

              case 'thinking.block':
                // Save thinking as a block
                if (data.task_id) {
                  const block: ThinkingBlock = {
                    id: `tb-${Date.now()}`,
                    timestamp: Date.now(),
                    content: data.thinking || data.text || '',
                  };
                  addThinkingBlock(data.task_id, block);
                }
                break;

              case 'tool.call_start':
                if (data.tool && data.tool.id) {
                  const tc: ToolCall = {
                    ...data.tool,
                    id: data.tool.id,
                    status: 'started',
                    start_time: new Date().toISOString(),
                  };
                  addToolCall(taskId, tc);
                  toolCallBufferRef.current[tc.id] = tc;
                }
                break;

              case 'tool.call_delta':
                if (data.tool && data.tool.id) {
                  const toolId = data.tool.id;
                  const existing = toolCallBufferRef.current[toolId];
                  if (existing) {
                    const updated: ToolCall = {
                      ...existing,
                      arguments: existing.arguments + (data.tool.arguments || ''),
                    };
                    updateToolCall(taskId, toolId, { arguments: updated.arguments });
                    toolCallBufferRef.current[toolId] = updated;
                  }
                }
                break;

              case 'tool.call_end':
                if (data.tool) {
                  updateToolCall(taskId, data.tool.id, {
                    arguments: data.tool.arguments,
                  });
                }
                break;

              case 'tool.result':
                if (data.tool) {
                  updateToolCall(taskId, data.tool.id, {
                    result: data.tool.result,
                    status: 'completed',
                    end_time: new Date().toISOString(),
                  });
                }
                break;

              case 'task.status':
                if (data.status?.state === 'completed') {
                  setStreaming(false);
                  // Save final assistant message if not already saved
                  const lastMessage = useChatStore.getState().messages[useChatStore.getState().messages.length - 1];
                  if (lastMessage?.role !== 'assistant') {
                    addMessage({
                      role: 'assistant',
                      content: '',
                      task_id: taskId,
                      timestamp: new Date().toISOString(),
                    });
                  }
                } else if (data.status?.state === 'failed') {
                  setStreaming(false);
                  const errorMsg = data.status.message as string || 'Task failed';
                  setError(errorMsg);
                  // Mark the last assistant message with the error so it's not just empty
                  // Use setState directly to avoid updateMessage matching user msg by task_id
                  useChatStore.setState((state) => {
                    const lastAssistantIdx = state.messages.map((m) => m.role).lastIndexOf('assistant');
                    if (lastAssistantIdx === -1) return state;
                    const assistantMsg = state.messages[lastAssistantIdx];
                    if (assistantMsg.content) return state;
                    const msgs = [...state.messages];
                    msgs[lastAssistantIdx] = { ...assistantMsg, content: `⚠️ ${errorMsg}` };
                    return { messages: msgs };
                  });
                }
                break;

              case 'error':
                setError(data.error || 'Unknown error');
                setStreaming(false);
                break;

              case 'subagent.started':
                if (data.subagent_id && data.tool_call_id) {
                  const subagent: TaskSession = {
                    id: data.subagent_id,
                    parent_context_id: '',
                    parent_tool_call_id: data.tool_call_id,
                    task: data.subagent_task || '',
                    context: '',
                    status: 'in_progress',
                    created_at: new Date().toISOString(),
                  };
                  setSubagent(data.tool_call_id, subagent);
                }
                break;

              case 'subagent.completed':
                if (data.subagent_id && data.tool_call_id) {
                  updateSubagent(data.tool_call_id, {
                    status: 'completed',
                    result: data.result || '',
                  });
                }
                break;

              case 'subagent.error':
                if (data.subagent_id && data.tool_call_id) {
                  updateSubagent(data.tool_call_id, {
                    status: 'failed',
                    error: data.error || 'Unknown error',
                  });
                }
                break;
            }
          },
          onerror: (err) => {
            console.error('SSE error:', err);
            setError(err.message || 'Connection error');
            setStreaming(false);
          },
        });
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to send message');
        setStreaming(false);
      }
    },
    [
      agentName,
      disconnect,
      setAgentName,
      addMessage,
      updateMessage,
      setStreaming,
      setError,
      addToolCall,
      updateToolCall,
      addThinkingBlock,
      setSubagent,
      updateSubagent,
      appendToLastMessage,
    ]
  );

  // Load context history
  const loadContext = useCallback(
    async (contextId: string) => {
      setContextId(contextId);
      try {
        const [ctxRes, subRes] = await Promise.all([
          fetch(`/api/contexts/${contextId}`),
          api.listSubagents(contextId),
        ]);
        if (!ctxRes.ok) throw new Error('Failed to load context');
        const data = await ctxRes.json();
        setMessages(data.messages || []);

        // Restore subagents from API
        if (subRes?.subagents) {
          const subagentMap: Record<string, TaskSession> = {};
          for (const s of subRes.subagents) {
            if (s.parent_tool_call_id) {
              subagentMap[s.parent_tool_call_id] = s as TaskSession;
            }
          }
          setSubagents(subagentMap);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load messages');
      }
    },
    [setContextId, setMessages, setError, setSubagents]
  );

  return {
    sendMessage,
    disconnect,
    loadContext,
    isStreaming: useChatStore((state) => state.isStreaming),
    messages: useChatStore((state) => state.messages),
    error: useChatStore((state) => state.error),
    contextId: useChatStore((state) => state.contextId),
  };
}