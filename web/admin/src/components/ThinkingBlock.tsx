import { useState } from 'react';
import { ChevronRight, ChevronDown, Brain } from 'lucide-react';
import type { ThinkingBlock } from '../types/chat';

interface ThinkingBlockProps {
  blocks: ThinkingBlock[];
  defaultExpanded?: boolean;
}

export default function ThinkingBlock({ blocks, defaultExpanded = false }: ThinkingBlockProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [activeBlock, setActiveBlock] = useState<string | null>(null);

  if (!blocks || blocks.length === 0) {
    return null;
  }

  const totalDuration = blocks.reduce((sum, b) => sum + (b.duration_ms || 0), 0);

  return (
    <div className="mb-4">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors cursor-pointer px-2 py-1 rounded hover:bg-gray-200 dark:hover:bg-gray-800"
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <span className="font-medium">Thinking ({blocks.length} step{blocks.length > 1 ? 's' : ''})</span>
        {totalDuration > 0 && (
          <span className="text-gray-500 dark:text-gray-400">· {totalDuration}ms</span>
        )}
      </button>

      {expanded && (
        <div className="mt-2 space-y-2">
          {blocks.map((block) => (
            <div
              key={block.id}
              className="bg-gray-200 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg overflow-hidden"
            >
              <button
                onClick={() => setActiveBlock(activeBlock === block.id ? null : block.id)}
                className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-gray-300 dark:hover:bg-gray-700 transition-colors"
              >
                <Brain size={14} className="text-gray-500 dark:text-gray-400" />
                <span className="text-xs font-mono text-gray-500 dark:text-gray-400">
                  {formatTime(block.timestamp)}
                </span>
                {block.duration_ms && block.duration_ms > 0 && (
                  <span className="text-xs text-gray-500 dark:text-gray-400">+{block.duration_ms}ms</span>
                )}
              </button>

              <div className="px-3 pb-3 text-sm text-gray-700 dark:text-gray-300 font-mono whitespace-pre-wrap">
                {block.content}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function formatTime(timestamp: number): string {
  const date = new Date(timestamp);
  return date.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}