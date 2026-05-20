import { useEffect, useState } from 'react';
import { MessageSquare, Trash2, Plus, Sparkles, Clock } from 'lucide-react';
import { api } from '../api/client';
import type { ContextListItem } from '../types/chat';

interface ContextPanelProps {
  agentName: string;
  currentContextId: string | null;
  onSelectContext: (id: string) => void;
  onNewContext: () => void;
  onDeleteContext: (id: string) => void;
}

export default function ContextPanel({
  agentName,
  currentContextId,
  onSelectContext,
  onNewContext,
  onDeleteContext,
}: ContextPanelProps) {
  const [contexts, setContexts] = useState<ContextListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  useEffect(() => {
    loadContexts();
  }, [agentName]);

  const loadContexts = async () => {
    setLoading(true);
    try {
      const data = await api.listContexts(agentName);
      setContexts(data.items || []);
    } catch (err) {
      console.error('Failed to load contexts:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    try {
      await api.deleteContext(id);
      loadContexts();
      if (currentContextId === id) {
        onNewContext();
      }
    } catch (err) {
      console.error('Failed to delete context:', err);
      alert('Failed to delete conversation');
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="w-64 border-r border-gray-300 dark:border-gray-700 bg-gray-200 dark:bg-gray-800 flex flex-col">
      <div className="p-4 border-b border-gray-300 dark:border-gray-700">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">Conversations</h2>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="p-4 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <div className="animate-spin">
              <div className="w-3 h-3 border border-2 border-purple-500 border-t-transparent rounded-full" />
            </div>
            Loading...
          </div>
        ) : contexts.length === 0 ? (
          <div className="p-4 text-xs text-gray-500 dark:text-gray-400">
            <div className="flex flex-col items-center justify-center gap-2 py-8">
              <Sparkles size={24} className="text-gray-400 dark:text-gray-500 opacity-50" />
              <p>No conversations yet</p>
              <p className="text-[10px]">Start chatting to create one</p>
            </div>
          </div>
        ) : (
          <div className="p-2 space-y-1">
            {contexts.map((ctx) => (
              <ContextItem
                key={ctx.id}
                context={ctx}
                isSelected={ctx.id === currentContextId}
                isDeleting={deletingId === ctx.id}
                onSelect={() => onSelectContext(ctx.id)}
                onDelete={() => handleDelete(ctx.id)}
              />
            ))}
          </div>
        )}
      </div>

      <div className="p-4 border-t border-gray-300 dark:border-gray-700">
        <button
          onClick={onNewContext}
          className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm bg-purple-500 text-white rounded-lg hover:bg-purple-600 transition-colors shadow-sm"
        >
          <Plus size={14} />
          New Conversation
        </button>
      </div>
    </div>
  );
}

interface ContextItemProps {
  context: ContextListItem;
  isSelected: boolean;
  isDeleting: boolean;
  onSelect: () => void;
  onDelete: () => void;
}

function ContextItem({ context, isSelected, isDeleting, onSelect, onDelete }: ContextItemProps) {
  const timeAgo = getTimeAgo(new Date(context.updated_at));

  return (
    <button
      onClick={onSelect}
      disabled={isDeleting}
      className={`
        group relative w-full text-left p-3 rounded-xl transition-all duration-200
        ${isSelected
          ? 'bg-purple-500 text-white shadow-md'
          : 'bg-gray-300 dark:bg-gray-700 text-gray-900 dark:text-gray-100 hover:bg-gray-400 dark:hover:bg-gray-600 hover:shadow-sm'
        }
        ${isDeleting ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}
      `}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <MessageSquare
              size={14}
              className={isSelected ? 'text-white' : 'text-gray-500 dark:text-gray-400'}
            />
            <span className="text-xs font-medium truncate">{context.title || 'New Chat'}</span>
          </div>
          <div className="flex items-center gap-2 text-xs opacity-70">
            <Clock size={10} />
            <span>{timeAgo}</span>
          </div>
        </div>

        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          disabled={isDeleting}
          className={`
            opacity-0 group-hover:opacity-100 p-1.5 rounded-lg hover:bg-red-100 dark:hover:bg-red-900/30 hover:text-red-500 transition-all
            ${isDeleting ? 'opacity-50' : ''}
          `}
        >
          {isDeleting ? (
            <div className="w-3 h-3 border-2 border-red-500 border-t-transparent rounded-full animate-spin" />
          ) : (
            <Trash2 size={12} />
          )}
        </button>
      </div>

      {context.message_count > 0 && (
        <div className={isSelected ? 'text-white/60' : 'text-gray-500 dark:text-gray-400'}>
          <span className="text-[10px]">{context.message_count} message{context.message_count > 1 ? 's' : ''}</span>
        </div>
      )}
    </button>
  );
}

function getTimeAgo(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}