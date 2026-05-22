import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Bot, ListTodo, Activity, Clock } from 'lucide-react'
import { api, Agent, Task, HealthResponse } from '../api/client'

function StatCard({ icon: Icon, label, value, sub }: { icon: typeof Bot; label: string; value: string | number; sub?: string }) {
  return (
    <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-5">
      <div className="flex items-center gap-2 text-[var(--text-tertiary)] mb-3">
        <Icon size={14} />
        <span className="text-xs uppercase tracking-wider">{label}</span>
      </div>
      <div className="text-2xl font-semibold text-[var(--text-primary)]">{value}</div>
      {sub && <div className="text-xs text-[var(--text-tertiary)] mt-1">{sub}</div>}
    </div>
  )
}

export default function Dashboard() {
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [error, setError] = useState('')
  const location = useLocation()

  useEffect(() => {
    setError('')
    Promise.all([
      api.getHealth().catch(() => null),
      api.listAgents().catch(() => []),
      api.listTasks({ size: 5 }).catch(() => ({ items: [] })),
    ]).then(([h, a, t]) => {
      if (h) setHealth(h)
      else setError('Cannot connect to API')
      setAgents(Array.isArray(a) ? a : [])
      setTasks((t as { items: Task[] }).items || [])
    })
  }, [location.key])

  if (error) {
    return (
      <div className="p-8">
        <div className="bg-[var(--bg-secondary)] border border-[var(--error)]/30 rounded-lg p-6 text-center">
          <p className="text-[var(--error)]">{error}</p>
          <p className="text-xs text-[var(--text-tertiary)] mt-2">Ensure the A2A platform server is running on the configured port</p>
        </div>
      </div>
    )
  }

  const connectedCount = agents.filter(a => a.status === 'connected').length

  return (
    <div className="p-8 max-w-5xl">
      <h2 className="text-lg font-semibold mb-6">Overview</h2>

      <div className="grid grid-cols-4 gap-4 mb-8">
        <StatCard icon={Bot} label="Agents" value={health?.agents_total ?? agents.length} sub={`${connectedCount} connected`} />
        <StatCard icon={ListTodo} label="Tasks" value={tasks.length > 0 ? `${tasks.length}+` : '0'} sub="recent" />
        <StatCard icon={Activity} label="Status" value={health?.status === 'ok' ? 'Healthy' : 'Unknown'} sub={`DB: ${health?.db || '-'}`} />
        <StatCard icon={Clock} label="Connected" value={health?.agents_connected ?? 0} sub="agents online" />
      </div>

      <div className="grid grid-cols-2 gap-6">
        <div>
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-medium text-[var(--text-secondary)]">Agents</h3>
            <Link to="/agents" className="text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">View all</Link>
          </div>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
            {agents.length === 0 ? (
              <div className="p-4 text-sm text-[var(--text-tertiary)]">No agents registered</div>
            ) : (
              agents.slice(0, 5).map(a => (
                <Link key={a.name} to={`/agents/${a.name}`} className="flex items-center justify-between p-3 hover:bg-[var(--bg-tertiary)]/50 transition-colors no-underline">
                  <div>
                    <div className="text-sm text-[var(--text-primary)]">{a.name}</div>
                    <div className="text-xs text-[var(--text-tertiary)]">{a.description || a.url}</div>
                  </div>
                  <span className={`text-xs px-2 py-0.5 rounded-full ${a.status === 'connected' ? 'bg-[var(--success)]/10 text-[var(--success)]' : 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'}`}>
                    {a.status || 'unknown'}
                  </span>
                </Link>
              ))
            )}
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-medium text-[var(--text-secondary)]">Recent Tasks</h3>
            <Link to="/tasks" className="text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">View all</Link>
          </div>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
            {tasks.length === 0 ? (
              <div className="p-4 text-sm text-[var(--text-tertiary)]">No tasks yet</div>
            ) : (
              tasks.slice(0, 5).map(t => (
                <Link key={t.local_task_id} to={`/tasks/${t.local_task_id}`} className="flex items-center justify-between p-3 hover:bg-[var(--bg-tertiary)]/50 transition-colors no-underline">
                  <div>
                    <div className="text-sm text-[var(--text-primary)] font-mono">{t.display_id || t.local_task_id.slice(0, 8)}</div>
                    <div className="text-xs text-[var(--text-tertiary)]">{t.source_agent || 'unknown'} → {t.target_agent || t.agent_name}</div>
                  </div>
                  <span className={`text-xs px-2 py-0.5 rounded-full ${
                    t.state === 'RESPONDED' ? 'bg-[var(--success)]/10 text-[var(--success)]' :
                    t.state === 'FAILED' ? 'bg-[var(--error)]/10 text-[var(--error)]' :
                    'bg-[var(--warning)]/10 text-[var(--warning)]'
                  }`}>
                    {t.state}
                  </span>
                </Link>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
