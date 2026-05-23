import { useMemo, useState } from 'react';
import MarkdownRenderer from './MarkdownRenderer';
import ThinkingBlock from './ThinkingBlock';
import ToolCallCard from './ToolCallCard';
import SubagentCard from './SubagentCard';
import { useChatStore } from '../stores/chatStore';
import { User, Bot, Clock, ChevronDown, ChevronRight, Brain } from 'lucide-react';
import type { ChatMessage, ThinkingBlock as ThinkingBlockType, ToolCall as ToolCallType } from '../types/chat';

interface MessageTimelineProps {
  messages: ChatMessage[];
}

export default function MessageTimeline({ messages }: MessageTimelineProps) {
  const groupedMessages = useMemo(() => {
    const groups: Array<{
      type: 'user' | 'assistant';
      message: ChatMessage;
    }> = [];

    messages.forEach((msg) => {
      if (msg.role === 'user' || msg.role === 'assistant') {
        if (msg.role === 'assistant' && !hasVisibleAssistantContent(msg)) return;
        groups.push({ type: msg.role, message: msg });
      }
    });

    return groups;
  }, [messages]);

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5">
      {groupedMessages.map((item, index) => (
        <MessageItem key={item.message.id || `${item.message.task_id || item.type}-${index}`} item={item} />
      ))}
    </div>
  );
}

function hasVisibleAssistantContent(message: ChatMessage) {
  if (message.content?.trim()) return true;
  if (message.reasoning_content?.trim()) return true;
  if (hasStructuredContent(message.tool_calls)) return true;
  if (hasStructuredContent(message.thinking_blocks)) return true;
  return false;
}

function hasStructuredContent(value: unknown) {
  if (!value) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    return trimmed !== '' && trimmed !== '[]' && trimmed !== 'null';
  }
  return true;
}

interface MessageItemProps {
  item: {
    type: 'user' | 'assistant';
    message: ChatMessage;
  };
}

function ToolCallWithSubagent({ tool }: { tool: ToolCallType }) {
  const subagent = useChatStore((state) => state.subagents[tool.id]);

  if (tool.name === 'spawn_agent' && subagent) {
    return <SubagentCard subagent={subagent} />;
  }

  return <ToolCallCard tool={tool} />;
}

function MessageItem({ item }: MessageItemProps) {
  const { message } = item;
  const isUser = item.type === 'user';

  // Parse thinking blocks
  const thinkingBlocks = useMemo(() => {
    if (!message.thinking_blocks) return [];
    if (typeof message.thinking_blocks === 'string') {
      try {
        return JSON.parse(message.thinking_blocks);
      } catch {
        return [];
      }
    }
    return message.thinking_blocks;
  }, [message.thinking_blocks]);

  // Parse tool calls
  const toolCalls = useMemo(() => {
    if (!message.tool_calls) return [];
    if (typeof message.tool_calls === 'string') {
      try {
        return JSON.parse(message.tool_calls);
      } catch {
        return [];
      }
    }
    return message.tool_calls;
  }, [message.tool_calls]);

  return (
    <div className={`flex w-full min-w-0 items-end gap-3 ${isUser ? 'justify-end' : 'justify-start'}`}>
      {!isUser && (
        <Avatar isUser={false} />
      )}

      <div
        className={`min-w-0 overflow-hidden rounded-2xl px-4 py-3 shadow-sm ${
          isUser
            ? 'max-w-[min(620px,75%)] rounded-br-md bg-orange-500 text-white'
            : 'max-w-[min(760px,82%)] rounded-bl-md border border-gray-200 bg-white text-gray-900 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100'
        }`}
      >
        {message.reasoning_content && !isUser && (
          <ReasoningDisclosure content={message.reasoning_content} />
        )}

        {thinkingBlocks.length > 0 && !isUser && (
          <div className="mb-3">
            <ThinkingBlock blocks={thinkingBlocks as ThinkingBlockType[]} />
          </div>
        )}

        {toolCalls.length > 0 && !isUser && (
          <div className="mb-3 space-y-2">
            {toolCalls.map((tool: ToolCallType) => (
              <ToolCallWithSubagent key={tool.id} tool={tool} />
            ))}
          </div>
        )}

        {message.content && (
          <MarkdownRenderer content={message.content} inverted={isUser} />
        )}

        {message.timestamp && (
          <div className={`mt-2 flex items-center gap-1 text-[11px] ${isUser ? 'text-white/70' : 'text-gray-500 dark:text-gray-400'}`}>
            <Clock size={10} />
            {new Date(message.timestamp).toLocaleTimeString()}
          </div>
        )}
      </div>

      {isUser && (
        <Avatar isUser />
      )}
    </div>
  );
}

function ReasoningDisclosure({ content }: { content: string }) {
  const [expanded, setExpanded] = useState(false);
  const preview = content.replace(/\s+/g, ' ').trim();

  return (
    <div className="mb-3 min-w-0 max-w-full overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900/60">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full min-w-0 items-center gap-2 overflow-hidden px-3 py-2 text-left text-xs text-gray-600 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
      >
        <span className="shrink-0">{expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}</span>
        <Brain size={14} className="shrink-0" />
        <span className="shrink-0 font-medium">Reasoning</span>
        {!expanded && preview && (
          <span className="min-w-0 flex-1 truncate text-gray-500 dark:text-gray-500">
            {preview}
          </span>
        )}
      </button>
      {expanded && (
        <div className="break-words border-t border-gray-200 px-3 py-2 font-mono text-xs leading-relaxed text-gray-700 whitespace-pre-wrap dark:border-gray-700 dark:text-gray-300">
          {content}
        </div>
      )}
    </div>
  );
}

function Avatar({ isUser }: { isUser: boolean }) {
  return (
    <div
      className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-white ${
        isUser ? 'bg-slate-600' : 'bg-orange-500'
      }`}
    >
      {isUser ? <User size={15} /> : <Bot size={15} />}
    </div>
  );
}
