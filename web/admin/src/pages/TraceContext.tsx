import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, Task } from '../api/client'

const NONE_CONTEXT = 'none'

function stateColor(state: string) {
  switch (state) {
    case 'RESPONDED':
    case 'COMPLETED':
      return 'bg-[var(--success)]/10 text-[var(--success)]'
    case 'FAILED':
    case 'ERROR':
      return 'bg-[var(--error)]/10 text-[var(--error)]'
    case 'WORKING':
    case 'STREAMING':
      return 'bg-[var(--info)]/10 text-[var(--info)]'
    default:
      return 'bg-[var(--warning)]/10 text-[var(--warning)]'
  }
}

export default function TraceContext() {
  const { contextId = NONE_CONTEXT } = useParams<{ contextId: string }>()
  const actualContextId = useMemo(() => {
    if (contextId === NONE_CONTEXT) return ''
    return decodeURIComponent(contextId)
  }, [contextId])
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [page, setPage] = useState(1)

  const load = (p = page) => {
    setLoading(true)
    setError('')
    api.listTasks({ context_id: actualContextId, page: p, size: 20 })
      .then(data => {
        setTasks(data.items || [])
        setTotal(data.total || 0)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load tasks'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load(page)
  }, [actualContextId, page])

  return (
    <div className="p-8 max-w-5xl">
      <div className="mb-6">
        <Link to="/traces" className="text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">
          Back to traces
        </Link>
        <div className="flex items-center justify-between mt-2">
          <div>
            <h2 className="text-lg font-semibold">Trace Session</h2>
            <p className="mt-1 font-mono text-xs text-[var(--text-tertiary)]">{actualContextId || 'None'}</p>
          </div>
          <span className="text-xs text-[var(--text-tertiary)]">{total} tasks</span>
        </div>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading tasks...</div>
      ) : (
        <>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Task</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Agent</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">State</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {tasks.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-6 text-center text-[var(--text-tertiary)]">No tasks found</td>
                  </tr>
                ) : (
                  tasks.map(task => (
                    <tr key={task.local_task_id} className="hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                      <td className="px-4 py-3">
                        <Link to={`/tasks/${task.local_task_id}`} className="font-mono text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">
                          {task.display_id || task.local_task_id.slice(0, 8)}
                        </Link>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-primary)]">{task.agent_name}</td>
                      <td className="px-4 py-3">
                        <span className={`text-xs px-2 py-0.5 rounded-full ${stateColor(task.state)}`}>{task.state}</span>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-tertiary)] text-xs">{new Date(task.created_at).toLocaleString()}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="flex items-center justify-between mt-4">
            <span className="text-xs text-[var(--text-tertiary)]">{total} total</span>
            <div className="flex gap-2">
              <button
                disabled={page === 1}
                onClick={() => setPage(p => p - 1)}
                className="px-3 py-1 text-xs bg-[var(--bg-secondary)] border border-[var(--border)] rounded text-[var(--text-secondary)] disabled:opacity-30"
              >
                Prev
              </button>
              <span className="px-3 py-1 text-xs text-[var(--text-tertiary)]">{page} / {Math.ceil(total / 20) || 1}</span>
              <button
                disabled={page * 20 >= total}
                onClick={() => setPage(p => p + 1)}
                className="px-3 py-1 text-xs bg-[var(--bg-secondary)] border border-[var(--border)] rounded text-[var(--text-secondary)] disabled:opacity-30"
              >
                Next
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
