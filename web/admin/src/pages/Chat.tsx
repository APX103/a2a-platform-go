import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { api } from '../api/client';
import { useChatStore } from '../stores/chatStore';
import { useChat } from '../hooks/useChat';
import ContextPanel from '../components/ContextPanel';
import ChatHeader from '../components/ChatHeader';
import MessageTimeline from '../components/MessageTimeline';
import InputBox from '../components/InputBox';

export default function Chat() {
  const { agentName } = useParams<{ agentName: string }>();
  const contextIdParam = new URLSearchParams(window.location.search).get('contextId');

  const { contextId, setContextId, setContexts, setAgentName, clearChat, setError } = useChatStore();
  const { sendMessage, loadContext, isStreaming, messages, error } = useChat(agentName || '');
  const [showSidebar, setShowSidebar] = useState(true);

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
    } else if (agentName) {
      loadContextList();
    }
  }, [contextIdParam, agentName]);

  // Load context list
  const loadContextList = async () => {
    try {
      const data = await api.listContexts(agentName || '');
      setContexts(data.items || []);
    } catch (err) {
      console.error('Failed to load contexts:', err);
    }
  };

  // Load existing context's messages
  useEffect(() => {
    if (contextId) {
      loadContext(contextId);
    }
  }, [contextId]);

  const handleSend = async (content: string) => {
    if (!agentName) return;

    if (!contextId) {
      try {
        const newCtx = await api.createContext({ agent_name: agentName, title: content.slice(0, 50) });
        setContextId(newCtx.id);
        loadContextList();
      } catch (err) {
        alert('Failed to create session');
        return;
      }
    }

    await sendMessage(content, contextId || undefined);
    loadContextList();
  };

  const handleNewContext = () => {
    setContextId(null);
    clearChat();
  };

  const handleDeleteContext = async () => {
    if (contextId) {
      await api.deleteContext(contextId);
      setContextId(null);
      clearChat();
      loadContextList();
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
    <div className="flex h-screen bg-white dark:bg-gray-900">
      {showSidebar && (
        <ContextPanel
          agentName={agentName}
          currentContextId={contextId}
          onSelectContext={handleSelectContext}
          onNewContext={handleNewContext}
          onDeleteContext={handleDeleteContext}
        />
      )}

      <div className="flex-1 flex flex-col">
        <ChatHeader
          agentName={agentName}
          contextId={contextId}
          onNewContext={handleNewContext}
          onDeleteContext={handleDeleteContext}
        />

        <div className="flex-1 overflow-y-auto bg-white dark:bg-gray-900 p-6">
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
            <div className="mx-6 mb-4 flex items-center gap-2 text-xs text-purple-500">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-purple-500 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-purple-500"></span>
              </span>
              AI is typing...
            </div>
          )}
        </div>

        <InputBox onSend={handleSend} disabled={isStreaming} placeholder={`Message ${agentName}...`} />
      </div>
    </div>
  );
}