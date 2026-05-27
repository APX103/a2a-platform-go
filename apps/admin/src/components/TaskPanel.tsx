import { useState } from 'react';
import { useChatStore } from '../stores/chatStore';
import { ChevronRight, ChevronDown, Loader2, CheckCircle, XCircle, Clock, Bot, PanelRightClose, PanelRightOpen } from 'lucide-react';
import type { TaskSession } from '../types/chat';

interface TaskPanelProps {
  contextId: string | null;
}

export default function TaskPanel({ contextId }: TaskPanelProps) {
  const subagents = useChatStore((state) => state.subagents);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [collapsed, setCollapsed] = useState(false);

  const tasks = Object.values(subagents);

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  if (collapsed) {
    return (
      <button
        onClick={() => setCollapsed(false)}
        className="h-full w-10 flex items-center justify-center border-l border-[var(--border)] bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)] text-[var(--text-secondary)]"
        title="Show task panel"
      >
        <PanelRightOpen size={18} />
      </button>
    );
  }

  return (
    <div className="w-80 border-l border-[var(--border)] bg-[var(--bg-secondary)] flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
        <h3 className="text-sm font-semibold text-[var(--text-primary)] flex items-center gap-2">
          <Bot size={14} />
          Tasks ({tasks.length})
        </h3>
        <button
          onClick={() => setCollapsed(true)}
          className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors"
          title="Hide task panel"
        >
          <PanelRightClose size={16} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        {tasks.length === 0 ? (
          <div className="text-xs text-[var(--text-tertiary)] text-center py-8">
            {contextId ? 'No tasks in this context yet.' : 'Select a context to view tasks.'}
          </div>
        ) : (
          tasks.map((task) => (
            <TaskCard key={task.id} task={task} expanded={expandedIds.has(task.id)} onToggle={() => toggleExpand(task.id)} />
          ))
        )}
      </div>
    </div>
  );
}

function TaskCard({ task, expanded, onToggle }: { task: TaskSession; expanded: boolean; onToggle: () => void }) {
  const statusConfig = {
    pending: {
      icon: <Clock size={12} className="text-blue-400" />,
      label: 'Pending',
      badgeClass: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
    },
    in_progress: {
      icon: <Loader2 size={12} className="animate-spin text-orange-500" />,
      label: 'In Progress',
      badgeClass: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
    },
    running: {
      icon: <Loader2 size={12} className="animate-spin text-orange-500" />,
      label: 'Running',
      badgeClass: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
    },
    completed: {
      icon: <CheckCircle size={12} className="text-green-500" />,
      label: 'Completed',
      badgeClass: 'bg-green-500/10 text-green-600 dark:text-green-400',
    },
    failed: {
      icon: <XCircle size={12} className="text-red-500" />,
      label: 'Failed',
      badgeClass: 'bg-red-500/10 text-red-600 dark:text-red-400',
    },
    timeout: {
      icon: <Clock size={12} className="text-gray-500" />,
      label: 'Timeout',
      badgeClass: 'bg-gray-500/10 text-gray-600 dark:text-gray-400',
    },
  };

  const config = statusConfig[task.status as keyof typeof statusConfig] || statusConfig.running;

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-primary)] overflow-hidden">
      <button
        onClick={onToggle}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-[var(--bg-tertiary)]/30 transition-colors"
      >
        {expanded ? <ChevronDown size={12} className="text-[var(--text-tertiary)]" /> : <ChevronRight size={12} className="text-[var(--text-tertiary)]" />}
        {config.icon}
        <span className="text-xs font-medium text-[var(--text-primary)] truncate flex-1">{task.task}</span>
        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${config.badgeClass}`}>
          {config.label}
        </span>
      </button>

      {expanded && (
        <div className="px-3 pb-3 space-y-2 border-t border-[var(--border)]">
          {task.subject && (
            <div className="pt-2">
              <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Subject</div>
              <div className="text-xs font-medium text-[var(--text-primary)]">{task.subject}</div>
            </div>
          )}
          {task.owner && (
            <div className="flex items-center gap-1">
              <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Owner:</div>
              <span className="text-xs text-[var(--text-secondary)]">{task.owner}</span>
            </div>
          )}
          {task.blocked_by && task.blocked_by.length > 0 && (
            <div className="flex items-center gap-1">
              <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Blocked By:</div>
              <span className="text-xs text-[var(--text-secondary)]">{task.blocked_by.join(', ')}</span>
            </div>
          )}
          <div>
            <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Task</div>
            <div className="text-xs text-[var(--text-secondary)] bg-[var(--bg-secondary)] border border-[var(--border)] rounded p-2">
              {task.task}
            </div>
          </div>

          {task.result && (
            <div>
              <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Result</div>
              <pre className="text-xs bg-[var(--bg-secondary)] border border-[var(--border)] rounded p-2 overflow-x-auto max-h-48 overflow-y-auto text-[var(--text-secondary)]">
                {task.result.length > 1000 ? task.result.slice(0, 1000) + '\n... (truncated)' : task.result}
              </pre>
            </div>
          )}

          {task.error && (
            <div>
              <div className="text-[10px] text-red-500 uppercase tracking-wider mb-1">Error</div>
              <div className="text-xs text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-800 rounded p-2">
                {task.error}
              </div>
            </div>
          )}

          {task.created_at && (
            <div className="text-[10px] text-[var(--text-tertiary)]">
              Created: {new Date(task.created_at).toLocaleString()}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
