import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Search } from 'lucide-react'
import { api, Task } from '../api/client'

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [filters, setFilters] = useState({ agent_name: '', state: '', search: '' })
  const [page, setPage] = useState(1)

  const load = (p = page) => {
    setLoading(true)
    api.listTasks({ ...filters, page: p, size: 20 })
      .then(d => { setTasks(d.items || []); setTotal(d.total || 0) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [page])

  const handleFilter = (e: React.FormEvent) => {
    e.preventDefault()
    setPage(1)
    load(1)
  }

  const stateColor = (state: string) => {
    switch (state) {
      case 'RESPONDED': case 'COMPLETED': return 'bg-[var(--success)]/10 text-[var(--success)]'
      case 'FAILED': return 'bg-[var(--error)]/10 text-[var(--error)]'
      case 'WORKING': case 'STREAMING': return 'bg-[var(--info)]/10 text-[var(--info)]'
      default: return 'bg-[var(--warning)]/10 text-[var(--warning)]'
    }
  }

  return (
    <div className="p-8 max-w-5xl">
      <h2 className="text-lg font-semibold mb-6">Tasks</h2>

      <form onSubmit={handleFilter} className="flex gap-3 mb-5">
        <div className="relative flex-1">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" />
          <input
            placeholder="Search tasks..."
            value={filters.search}
            onChange={e => setFilters(f => ({ ...f, search: e.target.value }))}
            className="w-full pl-8 pr-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
          />
        </div>
        <input
          placeholder="Agent name"
          value={filters.agent_name}
          onChange={e => setFilters(f => ({ ...f, agent_name: e.target.value }))}
          className="w-36 px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
        />
        <select
          value={filters.state}
          onChange={e => setFilters(f => ({ ...f, state: e.target.value }))}
          className="px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
        >
          <option value="">All states</option>
          <option value="PENDING">Pending</option>
          <option value="WORKING">Working</option>
          <option value="STREAMING">Streaming</option>
          <option value="RESPONDED">Responded</option>
          <option value="COMPLETED">Completed</option>
          <option value="FAILED">Failed</option>
        </select>
        <button type="submit" className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)]">
          Filter
        </button>
      </form>

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading...</div>
      ) : (
        <>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">ID</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Agent</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">State</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {tasks.length === 0 ? (
                  <tr><td colSpan={4} className="px-4 py-6 text-center text-[var(--text-tertiary)]">No tasks found</td></tr>
                ) : (
                  tasks.map((t: Task) => (
                    <tr key={t.local_task_id} className="hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                      <td className="px-4 py-3">
                        <Link to={`/tasks/${t.local_task_id}`} className="font-mono text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">
                          {t.display_id || t.local_task_id.slice(0, 8)}
                        </Link>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-primary)]">{t.agent_name}</td>
                      <td className="px-4 py-3">
                        <span className={`text-xs px-2 py-0.5 rounded-full ${stateColor(t.state)}`}>{t.state}</span>
                      </td>
                      <td className="px-4 py-3 text-[var(--text-tertiary)]">{new Date(t.created_at).toLocaleString()}</td>
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
