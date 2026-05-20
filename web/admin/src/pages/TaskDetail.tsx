import { useEffect, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { api, TaskDetail, Trace } from '../api/client'

export default function TaskDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [data, setData] = useState<TaskDetail | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    api.getTask(id)
      .then(d => setData(d))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return <div className="p-8 text-sm text-[var(--text-tertiary)]">Loading...</div>
  if (!data) return <div className="p-8 text-sm text-[var(--error)]">Task not found</div>

  const { task, messages, traces } = data

  const stateColor = (s: string) => {
    switch (s) {
      case 'RESPONDED': case 'COMPLETED': return 'text-[var(--success)]'
      case 'FAILED': return 'text-[var(--error)]'
      default: return 'text-[var(--warning)]'
    }
  }

  const parseTraceData = (tr: Trace) => {
    if (!tr.data_json) return null
    try { return JSON.parse(tr.data_json) } catch { return tr.data_json }
  }

  return (
    <div className="p-8 max-w-4xl">
      <button onClick={() => navigate('/tasks')} className="flex items-center gap-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-6 transition-colors">
        <ArrowLeft size={14} />
        Back to Tasks
      </button>

      <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-6 mb-6">
        <div className="flex items-start justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold font-mono">{task.display_id || task.local_task_id.slice(0, 8)}</h2>
            <p className="text-xs text-[var(--text-tertiary)] mt-0.5 font-mono">{task.local_task_id}</p>
          </div>
          <span className={`text-sm font-medium ${stateColor(task.state)}`}>{task.state}</span>
        </div>

        <div className="grid grid-cols-3 gap-4 text-sm">
          <div>
            <span className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Agent</span>
            <p className="text-[var(--text-primary)] mt-1">
              <Link to={`/agents/${task.agent_name}`} className="text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">{task.agent_name}</Link>
            </p>
          </div>
          <div>
            <span className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Created</span>
            <p className="text-[var(--text-primary)] mt-1">{new Date(task.created_at).toLocaleString()}</p>
          </div>
          {task.context_id && (
            <div>
              <span className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Context</span>
              <p className="mt-1 font-mono text-xs text-[var(--text-secondary)]">{task.context_id.slice(0, 12)}...</p>
            </div>
          )}
        </div>
      </div>

      {messages && messages.length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-3">Messages</h3>
          <div className="space-y-2">
            {messages.map((m, i) => (
              <div key={i} className={`bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4 ${m.role === 'user' ? 'border-l-2 border-l-[var(--info)]' : 'border-l-2 border-l-[var(--success)]'}`}>
                <div className="flex items-center gap-2 mb-2">
                  <span className={`text-xs font-medium uppercase ${m.role === 'user' ? 'text-[var(--info)]' : 'text-[var(--success)]'}`}>{m.role}</span>
                  {m.timestamp && <span className="text-xs text-[var(--text-tertiary)]">{new Date(m.timestamp).toLocaleTimeString()}</span>}
                </div>
                <p className="text-sm text-[var(--text-primary)] whitespace-pre-wrap">{m.content}</p>
              </div>
            ))}
          </div>
        </div>
      )}

      {traces && traces.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-3">Traces</h3>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
            {traces.map((tr, i) => {
              const parsed = parseTraceData(tr)
              return (
                <div key={i} className="px-4 py-3 text-sm">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="text-[var(--text-primary)] font-mono text-xs">{tr.event_type}</span>
                      {tr.agent_name && <span className="text-xs text-[var(--text-tertiary)]">{tr.agent_name}</span>}
                      {tr.target_agent && <span className="text-xs text-[var(--text-tertiary)]">→ {tr.target_agent}</span>}
                    </div>
                    <div className="flex items-center gap-3">
                      {tr.duration_ms != null && <span className="text-xs text-[var(--text-tertiary)]">{tr.duration_ms}ms</span>}
                      {tr.timestamp && <span className="text-xs text-[var(--text-tertiary)]">{new Date(tr.timestamp).toLocaleTimeString()}</span>}
                    </div>
                  </div>
                  {parsed && (
                    <pre className="mt-2 text-xs text-[var(--text-tertiary)] overflow-x-auto bg-[var(--bg-tertiary)] rounded p-2 max-h-32 overflow-y-auto">
                      {typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2)}
                    </pre>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
