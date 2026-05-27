import { CheckCircle, XCircle, Loader2, Wrench, ChevronRight } from 'lucide-react';
import { useState } from 'react';
import type { ToolCall } from '../types/chat';

interface ToolCallCardProps {
  tool: ToolCall;
}

export default function ToolCallCard({ tool }: ToolCallCardProps) {
  const [expanded, setExpanded] = useState(false);

  const statusIcon = {
    started: <Loader2 size={14} className="animate-spin text-orange-500" />,
    completed: <CheckCircle size={14} className="text-green-500" />,
    error: <XCircle size={14} className="text-red-500" />,
  }[tool.status] || <Wrench size={14} className="text-gray-500" />;

  const statusColor = {
    started: 'text-orange-500',
    completed: 'text-green-500',
    error: 'text-red-500',
  }[tool.status] || 'text-gray-500';

  const argsObj = tool.arguments_obj || parseArguments(tool.arguments);
  const elapsedSeconds = typeof tool.metadata?.elapsed_seconds === 'number' ? tool.metadata.elapsed_seconds : undefined;

  return (
    <div className="my-2 border border-gray-300 dark:border-gray-700 rounded-lg overflow-hidden bg-gray-200 dark:bg-gray-800">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-gray-300 dark:hover:bg-gray-700 transition-colors"
      >
        {statusIcon}
        <span className="text-xs font-mono font-semibold text-gray-900 dark:text-gray-100">{tool.name}</span>
        <span className={`text-xs ${statusColor}`}>{tool.status}</span>
        {tool.status === 'started' && elapsedSeconds !== undefined && (
          <span className="text-xs text-gray-500 dark:text-gray-400">{elapsedSeconds}s</span>
        )}
        <ChevronRight
          size={14}
          className={`ml-auto text-gray-500 transition-transform ${expanded ? 'rotate-90' : ''}`}
        />
      </button>

      {expanded && (
        <div className="px-3 pb-3 space-y-2">
          {/* Arguments */}
          <div>
            <div className="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Arguments</div>
            <pre className="text-xs text-gray-900 dark:text-gray-100 bg-white dark:bg-gray-900 border border-gray-300 dark:border-gray-700 rounded p-2 overflow-x-auto">
              {JSON.stringify(argsObj, null, 2)}
            </pre>
          </div>

          {/* Result */}
          {tool.result && (
            <div>
              <div className="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Result</div>
              <pre className="text-xs text-gray-900 dark:text-gray-100 bg-white dark:bg-gray-900 border border-gray-300 dark:border-gray-700 rounded p-2 overflow-x-auto max-h-48 overflow-y-auto">
                {tool.result.length > 2000 ? tool.result.slice(0, 2000) + '\n... (truncated)' : tool.result}
              </pre>
            </div>
          )}

          {/* Timing */}
          {(tool.start_time || tool.end_time) && (
            <div className="text-xs text-gray-500 dark:text-gray-400">
              {tool.start_time && <span>Started: {formatTime(tool.start_time)}</span>}
              {tool.end_time && <span className="ml-2">Ended: {formatTime(tool.end_time)}</span>}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function parseArguments(args: string): Record<string, unknown> {
  if (!args) return {};
  try {
    return JSON.parse(args);
  } catch {
    return { raw: args };
  }
}

function formatTime(time: string): string {
  const date = new Date(time);
  return date.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
