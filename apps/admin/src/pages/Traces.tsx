import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, TraceContextSummary } from '../api/client'

const NONE_CONTEXT = 'none'

function contextLabel(contextId: string) {
  return contextId || 'None'
}

function contextPath(contextId: string) {
  return `/traces/context/${contextId ? encodeURIComponent(contextId) : NONE_CONTEXT}`
}

export default function Traces() {
  const [contexts, setContexts] = useState<TraceContextSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.listTraceContexts()
      .then(data => setContexts(Array.isArray(data) ? data : []))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load traces'))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold">Traces</h2>
        <span className="text-xs text-[var(--text-tertiary)]">{contexts.length} root contexts</span>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading root contexts...</div>
      ) : (
        <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)]">
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Root Context</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Events</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Agents</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Last Active</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {contexts.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-6 text-center text-[var(--text-tertiary)]">No root contexts found</td>
                </tr>
              ) : (
                contexts.map(ctx => (
                  <tr key={ctx.context_id || NONE_CONTEXT} className="hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                    <td className="px-4 py-3">
                      <Link
                        to={contextPath(ctx.context_id)}
                        className="font-mono text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline"
                      >
                        {contextLabel(ctx.context_id)}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{ctx.trace_count}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {ctx.agents?.length ? ctx.agents.join(', ') : '-'}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-tertiary)] text-xs">
                      {ctx.last_active ? new Date(ctx.last_active).toLocaleString() : '-'}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
