import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Bot, Activity, Clock, GitBranch } from 'lucide-react'
import { api, Agent, Trace, HealthResponse } from '../api/client'

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
  const [traces, setTraces] = useState<Trace[]>([])
  const [error, setError] = useState('')
  const location = useLocation()

  useEffect(() => {
    Promise.all([
      api.getHealth().catch(() => null),
      api.listAgents().catch(() => []),
      api.listRecentTraces().catch(() => []),
    ]).then(([h, a, recentTraces]) => {
      if (h) setHealth(h)
      setError(h ? '' : 'Cannot connect to API')
      setAgents(Array.isArray(a) ? a : [])
      setTraces(Array.isArray(recentTraces) ? recentTraces : [])
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
  const formatTraceAgents = (trace: Trace) => {
    const from = trace.agent_name || 'unknown'
    return trace.target_agent ? `${from} → ${trace.target_agent}` : from
  }
  const formatTraceTime = (timestamp?: string) => {
    if (!timestamp) return 'unknown time'
    const date = new Date(timestamp)
    return Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString()
  }

  return (
    <div className="p-8 max-w-5xl">
      <h2 className="text-lg font-semibold mb-6">Overview</h2>

      <div className="grid grid-cols-3 gap-4 mb-8">
        <StatCard icon={Bot} label="Agents" value={health?.agents_total ?? agents.length} sub={`${connectedCount} connected`} />
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
            <h3 className="text-sm font-medium text-[var(--text-secondary)]">Recent Traces</h3>
            <Link to="/traces" className="text-xs text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">View all</Link>
          </div>
          <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
            {traces.length === 0 ? (
              <div className="p-4 text-sm text-[var(--text-tertiary)]">No traces yet</div>
            ) : (
              traces.slice(0, 5).map((trace, index) => (
                <Link
                  key={`${trace.timestamp || 'trace'}-${index}`}
                  to={`/traces/context/${encodeURIComponent(trace.root_context_id || trace.context_id || 'none')}`}
                  className="flex items-center justify-between gap-3 p-3 hover:bg-[var(--bg-tertiary)]/50 transition-colors no-underline"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm text-[var(--text-primary)]">
                      <GitBranch size={14} className="shrink-0 text-[var(--text-tertiary)]" />
                      <span className="truncate">{formatTraceAgents(trace)}</span>
                    </div>
                    <div className="text-xs text-[var(--text-tertiary)] mt-0.5">{formatTraceTime(trace.timestamp)}</div>
                  </div>
                  <span className="shrink-0 text-xs px-2 py-0.5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)]">
                    {trace.event_type || 'trace'}
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
