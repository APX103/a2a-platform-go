import { useState } from 'react';
import { ChevronRight, ChevronDown, Loader2, CheckCircle, XCircle, Clock, Bot } from 'lucide-react';
import type { SubagentSession } from '../types/chat';

interface SubagentCardProps {
  subagent: SubagentSession;
}

export default function SubagentCard({ subagent }: SubagentCardProps) {
  const [expanded, setExpanded] = useState(false);

  const statusConfig = {
    running: {
      icon: <Loader2 size={14} className="animate-spin text-orange-500" />,
      label: 'Running',
      badgeClass: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
      borderClass: 'border-orange-300 dark:border-orange-700',
      bgClass: 'bg-orange-50 dark:bg-orange-950/20',
    },
    completed: {
      icon: <CheckCircle size={14} className="text-green-500" />,
      label: 'Completed',
      badgeClass: 'bg-green-500/10 text-green-600 dark:text-green-400',
      borderClass: 'border-green-300 dark:border-green-700',
      bgClass: 'bg-green-50 dark:bg-green-950/20',
    },
    failed: {
      icon: <XCircle size={14} className="text-red-500" />,
      label: 'Failed',
      badgeClass: 'bg-red-500/10 text-red-600 dark:text-red-400',
      borderClass: 'border-red-300 dark:border-red-700',
      bgClass: 'bg-red-50 dark:bg-red-950/20',
    },
    timeout: {
      icon: <Clock size={14} className="text-gray-500" />,
      label: 'Timeout',
      badgeClass: 'bg-gray-500/10 text-gray-600 dark:text-gray-400',
      borderClass: 'border-gray-300 dark:border-gray-700',
      bgClass: 'bg-gray-50 dark:bg-gray-950/20',
    },
  };

  const config = statusConfig[subagent.status] || statusConfig.running;

  return (
    <div className={`my-2 rounded-lg border overflow-hidden ${config.borderClass} ${config.bgClass}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-black/5 dark:hover:bg-white/5 transition-colors"
      >
        {expanded ? <ChevronDown size={14} className="text-gray-500" /> : <ChevronRight size={14} className="text-gray-500" />}
        {config.icon}
        <Bot size={14} className="text-gray-500" />
        <span className="text-xs font-medium text-gray-700 dark:text-gray-300">Subagent</span>
        <span className="text-xs text-gray-500 dark:text-gray-400 truncate flex-1">{subagent.task}</span>
        <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${config.badgeClass}`}>
          {config.label}
        </span>
      </button>

      {expanded && (
        <div className="px-3 pb-3 space-y-2">
          <div>
            <div className="text-[10px] text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Task</div>
            <div className="text-xs text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded p-2">
              {subagent.task}
            </div>
          </div>

          {subagent.result && (
            <div>
              <div className="text-[10px] text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Result</div>
              <pre className="text-xs bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded p-2 overflow-x-auto max-h-48 overflow-y-auto text-gray-700 dark:text-gray-300">
                {subagent.result.length > 1000 ? subagent.result.slice(0, 1000) + '\n... (truncated)' : subagent.result}
              </pre>
            </div>
          )}

          {subagent.error && (
            <div>
              <div className="text-[10px] text-red-500 uppercase tracking-wider mb-1">Error</div>
              <div className="text-xs text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-800 rounded p-2">
                {subagent.error}
              </div>
            </div>
          )}

          {subagent.created_at && (
            <div className="text-[10px] text-gray-400 dark:text-gray-500">
              Created: {new Date(subagent.created_at).toLocaleString()}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
