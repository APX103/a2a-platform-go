import { ArrowLeft, MoreVertical, Trash2, Plus } from 'lucide-react';
import { useState } from 'react';

interface ChatHeaderProps {
  agentName: string;
  contextId: string | null;
  onNewContext: () => void;
  onDeleteContext: () => void;
}

export default function ChatHeader({ agentName, contextId, onNewContext, onDeleteContext }: ChatHeaderProps) {
  const [showMenu, setShowMenu] = useState(false);

  return (
    <div className="flex items-center justify-between px-6 py-4 border-b border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900">
      <div className="flex items-center gap-3">
        <a href="/" className="p-2 -ml-2 text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100 transition-colors rounded-lg hover:bg-gray-200 dark:hover:bg-gray-800">
          <ArrowLeft size={20} />
        </a>
        <div>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{agentName}</h1>
          {contextId && (
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">Session: {contextId.slice(0, 8)}</p>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2">
        <button
          onClick={onNewContext}
          className="flex items-center gap-1.5 px-3 py-2 text-sm bg-purple-500 text-white rounded-lg hover:bg-purple-600 transition-colors"
        >
          <Plus size={14} />
          New Chat
        </button>

        {contextId && (
          <div className="relative">
            <button
              onClick={() => setShowMenu(!showMenu)}
              className="p-2 text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100 hover:bg-gray-200 dark:hover:bg-gray-800 rounded-lg transition-colors"
            >
              <MoreVertical size={18} />
            </button>

            {showMenu && (
              <div className="absolute right-0 mt-2 w-48 bg-gray-200 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg shadow-lg z-10">
                <button
                  onClick={async (e) => {
                    e.stopPropagation();
                    if (confirm('Delete this conversation?')) {
                      await onDeleteContext();
                      setShowMenu(false);
                    }
                  }}
                  className="w-full flex items-center gap-2 px-4 py-2 text-sm text-left text-red-500 hover:bg-gray-300 dark:hover:bg-gray-700 transition-colors"
                >
                  <Trash2 size={14} />
                  Delete Conversation
                </button>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}