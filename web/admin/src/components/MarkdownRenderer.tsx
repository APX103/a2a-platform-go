import React, { useMemo, useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { createHighlighter, type BundledLanguage, type BundledTheme } from 'shiki';

interface MarkdownRendererProps {
  content: string;
  className?: string;
}

export default function MarkdownRenderer({ content, className = '' }: MarkdownRendererProps) {
  const [highlighter, setHighlighter] = useState<Awaited<ReturnType<typeof createHighlighter>> | null>(null);

  // Load Shiki highlighter on mount
  useEffect(() => {
    createHighlighter({
      themes: ['github-dark-dimmed'],
      langs: ['javascript', 'typescript', 'python', 'go', 'bash', 'json', 'markdown', 'text'],
    }).then(setHighlighter);
  }, []);

  const CodeBlock = useMemo(() => {
    return ({ node, inline, className, children, ...props }: any) => {
      const match = /language-(\w+)/.exec(className || '');
      const language = (match?.[1] || 'text') as BundledLanguage;

      if (!inline && highlighter) {
        try {
          const html = highlighter.codeToHtml(String(children).replace(/\n$/, ''), {
            lang: language,
            theme: 'github-dark-dimmed',
          });
          return (
            <div
              className="rounded-lg my-4 overflow-x-auto"
              dangerouslySetInnerHTML={{ __html: html }}
            />
          );
        } catch (e) {
          // Fallback for unsupported languages
        }
      }

      return (
        <code className={className} {...props}>
          {children}
        </code>
      );
    };
  }, [highlighter]);

  const components = useMemo(() => ({
    // Code blocks with syntax highlighting
    code: CodeBlock,

    // Inline code
    inlineCode({ node, inline, ...props }: any) {
      return (
        <code
          className="bg-gray-800 text-gray-200 px-1.5 py-0.5 rounded text-sm font-mono"
          {...props}
        />
      );
    },

    // Headings
    h1({ children }: any) {
      return <h1 className="text-2xl font-bold mt-6 mb-4 text-gray-900 dark:text-gray-100">{children}</h1>;
    },
    h2({ children }: any) {
      return <h2 className="text-xl font-bold mt-5 mb-3 text-gray-900 dark:text-gray-100">{children}</h2>;
    },
    h3({ children }: any) {
      return <h3 className="text-lg font-semibold mt-4 mb-2 text-gray-900 dark:text-gray-100">{children}</h3>;
    },
    h4({ children }: any) {
      return <h4 className="text-base font-semibold mt-3 mb-2 text-gray-900 dark:text-gray-100">{children}</h4>;
    },

    // Paragraphs
    p({ children }: any) {
      return <p className="mb-4 text-gray-700 dark:text-gray-300 leading-relaxed">{children}</p>;
    },

    // Lists
    ul({ children }: any) {
      return <ul className="mb-4 pl-6 space-y-1 text-gray-700 dark:text-gray-300 list-disc">{children}</ul>;
    },
    ol({ children }: any) {
      return <ol className="mb-4 pl-6 space-y-1 text-gray-700 dark:text-gray-300 list-decimal">{children}</ol>;
    },
    li({ children }: any) {
      return <li>{children}</li>;
    },

    // Blockquotes
    blockquote({ children }: any) {
      return (
        <blockquote className="pl-4 border-l-4 border-orange-500 my-4 text-gray-600 dark:text-gray-400 italic">
          {children}
        </blockquote>
      );
    },

    // Links
    a({ href, children }: any) {
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="text-orange-600 hover:underline dark:text-orange-400"
        >
          {children}
        </a>
      );
    },

    // Tables
    table({ children }: any) {
      return (
        <div className="my-4 overflow-x-auto">
          <table className="min-w-full border border-gray-300 dark:border-gray-600 rounded-lg">{children}</table>
        </div>
      );
    },
    thead({ children }: any) {
      return <thead className="bg-gray-200 dark:bg-gray-800">{children}</thead>;
    },
    tbody({ children }: any) {
      return <tbody>{children}</tbody>;
    },
    tr({ children }: any) {
      return <tr className="border-b border-gray-300 dark:border-gray-600 last:border-0">{children}</tr>;
    },
    th({ children }: any) {
      return (
        <th className="px-4 py-2 text-left text-sm font-semibold text-gray-900 dark:text-gray-100">{children}</th>
      );
    },
    td({ children }: any) {
      return <td className="px-4 py-2 text-sm text-gray-700 dark:text-gray-300">{children}</td>;
    },

    // Horizontal rule
    hr() {
      return <hr className="my-6 border-gray-300 dark:border-gray-600" />;
    },

    // Strong/bold
    strong({ children }: any) {
      return <strong className="font-semibold text-gray-900 dark:text-gray-100">{children}</strong>;
    },

    // Italic
    em({ children }: any) {
      return <em className="italic text-gray-700 dark:text-gray-300">{children}</em>;
    },
  }), [highlighter, CodeBlock]);

  return (
    <div className={`prose prose-sm max-w-none ${className}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={components as any}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}