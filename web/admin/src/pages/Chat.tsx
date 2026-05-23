import { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import { api } from '../api/client';
import { useChatStore } from '../stores/chatStore';
import { useChat } from '../hooks/useChat';
import ContextPanel from '../components/ContextPanel';
import ChatHeader from '../components/ChatHeader';
import MessageTimeline from '../components/MessageTimeline';
import InputBox from '../components/InputBox';
import TaskPanel from '../components/TaskPanel';

export default function Chat() {
  const { agentName } = useParams<{ agentName: string }>();
  const searchParams = new URLSearchParams(window.location.search);
  const contextIdParam = searchParams.get('contextId');
  const groupIdParam = searchParams.get('groupId') || undefined;
  const draftParam = searchParams.get('draft') || '';
  const autoSendParam = searchParams.get('autoSend') === '1';

  const { contextId, setContextId, setContexts, setAgentName, clearChat, setError } = useChatStore();
  const { sendMessage, loadContext, isStreaming, messages, error } = useChat(agentName || '');
  const [showSidebar, setShowSidebar] = useState(true);
  const [showTaskPanel, setShowTaskPanel] = useState(true);
  const autoSentRef = useRef(false);

  // Initialize agent name from URL
  useEffect(() => {
    if (agentName) {
      setAgentName(agentName);
    }
  }, [agentName, setAgentName]);

  // Load context from URL param
  useEffect(() => {
    if (contextIdParam) {
      setContextId(contextIdParam);
      loadContext(contextIdParam);
    }
  }, [contextIdParam, setContextId, loadContext]);

  // Load context list
  const loadContextList = async () => {
    try {
      const data = await api.listContexts(agentName || '');
      setContexts(data.items || []);
    } catch (err) {
      console.error('Failed to load contexts:', err);
    }
  };

  const handleSend = async (content: string) => {
    if (!agentName) return;

    let currentContextId = contextId;
    // If no context, create one (fallback in case handleNewContext wasn't called)
    if (!currentContextId) {
      try {
        const newCtx = await api.createContext({ agent_name: agentName, title: content.slice(0, 50) });
        currentContextId = newCtx.id;
        setContextId(newCtx.id);
        clearChat();
        loadContextList();
      } catch (err) {
        alert('Failed to create session');
        return;
      }
    }

    // Update title to first message if it's still "New Chat"
    if (currentContextId) {
      const contexts = useChatStore.getState().contexts;
      const currentCtx = contexts.find(c => c.id === currentContextId);
      if (currentCtx?.title === 'New Chat') {
        api.updateContextTitle(currentContextId, content.slice(0, 50)).catch(console.error);
        loadContextList();
      }
    }

    await sendMessage(content, currentContextId || undefined, {
      groupId: groupIdParam,
      rootContextId: currentContextId || undefined,
    });
    loadContextList();
  };

  useEffect(() => {
    if (!autoSendParam || !draftParam || autoSentRef.current) return;
    if (contextIdParam && contextId !== contextIdParam) return;
    autoSentRef.current = true;
    const nextParams = new URLSearchParams(window.location.search);
    nextParams.delete('draft');
    nextParams.delete('autoSend');
    const nextQuery = nextParams.toString();
    window.history.replaceState(null, '', `${window.location.pathname}${nextQuery ? `?${nextQuery}` : ''}`);
    handleSend(draftParam);
  }, [autoSendParam, draftParam, contextIdParam, contextId, agentName]);

  const handleNewContext = async () => {
    if (!agentName) {
      setError('No agent selected');
      return;
    }
    try {
      const newCtx = await api.createContext({ agent_name: agentName, title: 'New Chat' });
      setContextId(newCtx.id);
      clearChat();
      loadContextList();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create conversation');
    }
  };

  const handleDeleteContext = async () => {
    if (contextId) {
      try {
        await api.deleteContext(contextId);
        setContextId(null);
        clearChat();
        loadContextList();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to delete conversation');
      }
    }
  };

  const handleSelectContext = async (id: string) => {
    setContextId(id);
    await loadContext(id);
  };

  if (!agentName) {
    return (
      <div className="flex items-center justify-center h-screen bg-white dark:bg-gray-900">
        <div className="text-gray-500 dark:text-gray-400">Invalid agent</div>
      </div>
    );
  }

  return (
    <div className="flex h-screen min-w-0 overflow-hidden bg-white dark:bg-gray-900">
      {showSidebar && (
        <ContextPanel
          agentName={agentName}
          currentContextId={contextId}
          onSelectContext={handleSelectContext}
          onNewContext={handleNewContext}
          onDeleteContext={handleDeleteContext}
        />
      )}

      <div className="min-w-0 flex-1 flex flex-col">
        <ChatHeader
          agentName={agentName}
          contextId={contextId}
          groupId={groupIdParam}
          onNewContext={handleNewContext}
          onDeleteContext={handleDeleteContext}
          onToggleTaskPanel={() => setShowTaskPanel(v => !v)}
          showTaskPanel={showTaskPanel}
        />

        <div className="min-w-0 flex-1 overflow-y-auto overflow-x-hidden bg-white dark:bg-gray-900 p-6">
          {messages.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-gray-500 dark:text-gray-400">
              <p className="mb-2">Start a conversation with {agentName}</p>
              <p className="text-sm">Type a message below to begin</p>
            </div>
          ) : (
            <MessageTimeline messages={messages} />
          )}

          {error && (
            <div className="mx-6 mb-4 p-3 bg-red-100 dark:bg-red-900/20 border border-red-300 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
              {error}
              <button onClick={() => setError(null)} className="ml-2 underline">Dismiss</button>
            </div>
          )}

          {isStreaming && (
            <div className="mx-6 mb-4 flex items-center gap-2 text-xs text-orange-500">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-500 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-orange-500"></span>
              </span>
              AI is typing...
            </div>
          )}
        </div>

        <InputBox onSend={handleSend} disabled={isStreaming} placeholder={`Message ${agentName}...`} />
      </div>

      {showTaskPanel && (
        <TaskPanel contextId={contextId} />
      )}
    </div>
  );
}
