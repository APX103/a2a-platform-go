import { useMemo } from 'react';
import MarkdownRenderer from './MarkdownRenderer';
import ThinkingBlock from './ThinkingBlock';
import ToolCallCard from './ToolCallCard';
import { User, Bot, Clock } from 'lucide-react';
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
        groups.push({ type: msg.role, message: msg });
      }
    });

    return groups;
  }, [messages]);

  return (
    <div className="relative">
      {/* Timeline line */}
      <div className="absolute left-4 top-0 bottom-0 w-0.5 bg-gray-300 dark:bg-gray-700" />

      <div className="space-y-6 pl-8">
        {groupedMessages.map((item, index) => (
          <MessageItem key={item.message.id || index} item={item} />
        ))}
      </div>
    </div>
  );
}

interface MessageItemProps {
  item: {
    type: 'user' | 'assistant';
    message: ChatMessage;
  };
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
    <div className="relative">
      {/* Avatar node */}
      <div
        className={`absolute left-[-34px] w-8 h-8 rounded-full flex items-center justify-center text-white text-xs ${
          isUser ? 'bg-purple-500' : 'bg-green-500'
        }`}
      >
        {isUser ? <User size={14} /> : <Bot size={14} />}
      </div>

      <div className={`rounded-lg p-4 ${isUser ? 'bg-purple-500 text-white ml-2' : 'bg-gray-200 dark:bg-gray-800 border border-gray-300 dark:border-gray-700'}`}>
        {/* Thinking blocks (before tool calls) */}
        {thinkingBlocks.length > 0 && <ThinkingBlock blocks={thinkingBlocks as ThinkingBlockType[]} />}

        {/* Tool calls (before content) */}
        {toolCalls.map((tool: ToolCallType) => (
          <ToolCallCard key={tool.id} tool={tool} />
        ))}

        {/* Message content */}
        {message.content && (
          <MarkdownRenderer content={message.content} className={isUser ? 'text-white' : ''} />
        )}

        {/* Timestamp */}
        {message.timestamp && (
          <div className={`mt-2 text-xs ${isUser ? 'text-white/60' : 'text-gray-500 dark:text-gray-400'} flex items-center gap-1`}>
            <Clock size={10} />
            {new Date(message.timestamp).toLocaleTimeString()}
          </div>
        )}
      </div>
    </div>
  );
}