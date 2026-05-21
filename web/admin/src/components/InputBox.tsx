import { useState, KeyboardEvent } from 'react';
import { Send, Square } from 'lucide-react';

interface InputBoxProps {
  onSend: (content: string) => void;
  disabled?: boolean;
  placeholder?: string;
}

export default function InputBox({ onSend, disabled = false, placeholder = 'Type a message...' }: InputBoxProps) {
  const [content, setContent] = useState('');

  const handleSend = () => {
    const trimmed = content.trim();
    if (trimmed && !disabled) {
      onSend(trimmed);
      setContent('');
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="p-4 border-t border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900">
      <div className="flex gap-3">
        <div className="flex-1 relative">
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
            disabled={disabled}
            rows={1}
            className="w-full px-4 py-3 bg-gray-200 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-xl text-sm text-gray-900 dark:text-gray-100 placeholder-gray-500 dark:placeholder-gray-400 resize-none outline-none focus:border-orange-500 disabled:opacity-50"
            style={{
              minHeight: '48px',
              maxHeight: '200px',
            }}
          />
        </div>
        <button
          onClick={handleSend}
          disabled={disabled || !content.trim()}
          className="self-end px-4 py-3 bg-orange-500 text-white rounded-xl hover:bg-orange-600 disabled:bg-gray-300 dark:disabled:bg-gray-700 disabled:text-gray-500 dark:disabled:text-gray-500 disabled:cursor-not-allowed transition-all flex items-center gap-2"
        >
          {disabled ? <Square size={16} /> : <Send size={16} />}
        </button>
      </div>
    </div>
  );
}