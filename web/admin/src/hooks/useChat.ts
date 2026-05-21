import { useCallback, useRef, useState } from 'react';
import { fetchEventSource } from '@microsoft/fetch-event-source';
import { useChatStore } from '../stores/chatStore';
import type { SSEEvent, ToolCall, ThinkingBlock } from '../types/chat';

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
    appendToLastMessage,
  } = useChatStore();

  const controllerRef = useRef<AbortController | null>(null);
  const [currentTaskId, setCurrentTaskId] = useState<string | null>(null);
  const [toolCallBuffer, setToolCallBuffer] = useState<Record<string, ToolCall>>({});
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
                  setToolCallBuffer((prev) => ({
                    ...prev,
                    [tc.id]: tc,
                  }));
                }
                break;

              case 'tool.call_delta':
                if (data.tool && data.tool.id) {
                  const toolId = data.tool.id;
                  const existing = toolCallBuffer[toolId];
                  if (existing) {
                    const updated: ToolCall = {
                      ...existing,
                      arguments: existing.arguments + (data.tool.arguments || ''),
                    };
                    updateToolCall(taskId, toolId, { arguments: updated.arguments });
                    setToolCallBuffer((prev) => ({
                      ...prev,
                      [toolId]: updated,
                    }));
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
                  setError(data.status.message as string || 'Task failed');
                }
                break;

              case 'error':
                setError(data.error || 'Unknown error');
                setStreaming(false);
                break;

              case 'subagent.started':
                // Subagent spawned, could show in UI
                break;

              case 'subagent.completed':
                // Subagent finished
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
      appendToLastMessage,
      toolCallBuffer,
    ]
  );

  // Load context history
  const loadContext = useCallback(
    async (contextId: string) => {
      setContextId(contextId);
      try {
        const response = await fetch(`/api/contexts/${contextId}`);
        if (!response.ok) throw new Error('Failed to load context');
        const data = await response.json();
        setMessages(data.messages || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load messages');
      }
    },
    [setContextId, setMessages, setError]
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