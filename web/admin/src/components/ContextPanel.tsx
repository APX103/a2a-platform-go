import { useEffect, useState } from 'react';
import { MessageSquare, Trash2, Plus } from 'lucide-react';
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
    if (!confirm('Delete this conversation?')) return;
    try {
      await api.deleteContext(id);
      loadContexts();
      if (currentContextId === id) {
        onNewContext();
      }
    } catch (err) {
      console.error('Failed to delete context:', err);
      alert('Failed to delete conversation');
    }
  };

  return (
    <div className="w-64 border-r border-gray-300 dark:border-gray-700 bg-gray-200 dark:bg-gray-800 flex flex-col">
      <div className="p-4 border-b border-gray-300 dark:border-gray-700">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">Conversations</h2>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="p-4 text-xs text-gray-500 dark:text-gray-400">Loading...</div>
        ) : contexts.length === 0 ? (
          <div className="p-4 text-xs text-gray-500 dark:text-gray-400">No conversations yet</div>
        ) : (
          <div className="p-2 space-y-1">
            {contexts.map((ctx) => (
              <ContextItem
                key={ctx.id}
                context={ctx}
                isSelected={ctx.id === currentContextId}
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
          className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm bg-purple-500 text-white rounded-lg hover:bg-purple-600 transition-colors"
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
  onSelect: () => void;
  onDelete: () => void;
}

function ContextItem({ context, isSelected, onSelect, onDelete }: ContextItemProps) {
  return (
    <div
      onClick={onSelect}
      className={`group relative p-3 rounded-lg cursor-pointer transition-colors ${
        isSelected
          ? 'bg-purple-500 text-white'
          : 'bg-gray-300 dark:bg-gray-700 text-gray-900 dark:text-gray-100 hover:bg-gray-400 dark:hover:bg-gray-600'
      }`}
    >
      <div className="flex items-start justify-between">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <MessageSquare size={12} className={isSelected ? 'text-white' : 'text-gray-500 dark:text-gray-400'} />
            <span className="text-xs font-medium truncate">{context.title || 'New Chat'}</span>
          </div>
          <div className="text-xs opacity-70 truncate">{context.message_count} messages</div>
        </div>

        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          className="opacity-0 group-hover:opacity-100 p-1 hover:text-red-500 transition-all"
        >
          <Trash2 size={12} />
        </button>
      </div>

      <div className="mt-2 text-[10px] opacity-60">
        {new Date(context.updated_at).toLocaleDateString()}
      </div>
    </div>
  );
}