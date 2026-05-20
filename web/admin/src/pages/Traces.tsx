import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, Task, Trace } from '../api/client'

export default function Traces() {
  const [traces, setTraces] = useState<(Trace & { _taskId?: string })[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.listTasks({ size: 10 })
      .then(async (res) => {
        const allTraces: (Trace & { _taskId?: string })[] = []
        const taskIds = (res.items || []).slice(0, 10).map(t => t.local_task_id)
        const details = await Promise.all(taskIds.map(id => api.getTask(id).catch(() => null)))
        for (const d of details) {
          if (d && d.traces) {
            for (const tr of d.traces) {
              allTraces.push({ ...tr, _taskId: d.task.local_task_id })
            }
          }
        }
        allTraces.sort((a, b) => {
          const ta = a.timestamp ? new Date(a.timestamp).getTime() : 0
          const tb = b.timestamp ? new Date(b.timestamp).getTime() : 0
          return tb - ta
        })
        setTraces(allTraces)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="p-8 max-w-5xl">
      <h2 className="text-lg font-semibold mb-6">Traces</h2>

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading traces from recent tasks...</div>
      ) : (
        <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)]">
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Event</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Agent</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Target</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Task</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Duration</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {traces.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-6 text-center text-[var(--text-tertiary)]">No traces found</td></tr>
              ) : (
                traces.map((tr, i) => (
                  <tr key={i} className="hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                    <td className="px-4 py-3 font-mono text-xs text-[var(--text-primary)]">{tr.event_type}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{tr.agent_name || '-'}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{tr.target_agent || '-'}</td>
                    <td className="px-4 py-3">
                      {tr._taskId ? (
                        <Link to={`/tasks/${tr._taskId}`} className="font-mono text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">
                          {tr._taskId.slice(0, 8)}
                        </Link>
                      ) : '-'}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-tertiary)]">{tr.duration_ms != null ? `${tr.duration_ms}ms` : '-'}</td>
                    <td className="px-4 py-3 text-[var(--text-tertiary)] text-xs">{tr.timestamp ? new Date(tr.timestamp).toLocaleTimeString() : '-'}</td>
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
