import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Trash2, Save } from 'lucide-react'
import { api, Agent, AgentCard } from '../api/client'
import { safeStorage } from '../utils/storage'

function formatAgentCard(agent: Agent) {
  if (!agent.agent_card_json) {
    return JSON.stringify({
      name: agent.name,
      description: agent.description || '',
      version: agent.version || '1.0.0',
      url: agent.url || `/agent/${agent.name}`,
      skills: [],
      x_context_mode: agent.context_mode || 'context',
    }, null, 2)
  }
  try {
    return JSON.stringify(JSON.parse(agent.agent_card_json), null, 2)
  } catch {
    return agent.agent_card_json
  }
}

function skillLabel(skill: string | { id?: string; name?: string; description?: string }) {
  return typeof skill === 'string' ? skill : skill.name || skill.id || skill.description || 'skill'
}

export default function AgentDetail() {
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const [agent, setAgent] = useState<Agent | null>(null)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ url: '', context_mode: 'context', agent_card: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [token, setToken] = useState(() => safeStorage.getItem('admin_token'))

  useEffect(() => {
    if (!name) return
    api.getAgent(name)
      .then(a => {
        setAgent(a)
        setForm({ url: a.url, context_mode: a.context_mode || 'context', agent_card: formatAgentCard(a) })
      })
      .catch(() => setError('Agent not found'))
      .finally(() => setLoading(false))
  }, [name])

  const handleSave = async () => {
    if (!token || !name) {
      setError('Admin token required')
      return
    }
    try {
      const card = JSON.parse(form.agent_card) as AgentCard
      const updated = await api.updateAgent(name, {
        url: form.url,
        context_mode: form.context_mode,
        agent_card: card,
      }, token)
      setEditing(false)
      setAgent(updated)
      setForm({ url: updated.url, context_mode: updated.context_mode || 'context', agent_card: formatAgentCard(updated) })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleDelete = async () => {
    const token = prompt('Enter admin token to delete this agent:')
    if (!token || !name) return
    try {
      await api.deleteAgent(name, token)
      navigate('/agents')
    } catch (err) {
      setError(String(err))
    }
  }

  if (loading) return <div className="p-8 text-sm text-[var(--text-tertiary)]">Loading...</div>
  if (!agent) return <div className="p-8 text-sm text-[var(--error)]">{error || 'Not found'}</div>

  return (
    <div className="p-8 max-w-3xl">
      <button onClick={() => navigate('/agents')} className="flex items-center gap-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-6 transition-colors">
        <ArrowLeft size={14} />
        Back to Agents
      </button>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">{error}</div>
      )}

      <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-6">
        <div className="flex items-start justify-between mb-6">
          <div className="flex items-center gap-3">
            <div className={`w-3 h-3 rounded-full ${agent.status === 'connected' ? 'bg-[var(--success)]' : 'bg-[var(--text-tertiary)]'}`} />
            <div>
              <h2 className="text-xl font-semibold">{agent.name}</h2>
              <span className={`text-xs ${agent.status === 'connected' ? 'text-[var(--success)]' : 'text-[var(--text-tertiary)]'}`}>
                {agent.status || 'unknown'}
              </span>
            </div>
          </div>
          <div className="flex gap-2">
            {editing ? (
              <button onClick={handleSave} className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)]">
                <Save size={14} />
                Save
              </button>
            ) : (
              <button onClick={() => setEditing(true)} className="px-3 py-1.5 text-sm bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)]">
                Edit
              </button>
            )}
            <button onClick={handleDelete} className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--error)]/10 text-[var(--error)] rounded-md hover:bg-[var(--error)]/20">
              <Trash2 size={14} />
              Delete
            </button>
          </div>
        </div>

        <div className="space-y-4">
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Admin Token</label>
            <input
              type="password"
              value={token}
              onChange={e => { setToken(e.target.value); safeStorage.setItem('admin_token', e.target.value) }}
              className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
            />
          </div>
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Upstream URL</label>
            {editing ? (
              <input
                value={form.url}
                onChange={e => setForm(f => ({ ...f, url: e.target.value }))}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
              />
            ) : (
              <p className="text-sm text-[var(--text-primary)] mt-1 font-mono">{agent.url}</p>
            )}
          </div>
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Context Mode</label>
            {editing ? (
              <select
                value={form.context_mode}
                onChange={e => setForm(f => ({ ...f, context_mode: e.target.value }))}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
              >
                <option value="context">context</option>
                <option value="stateless">stateless</option>
              </select>
            ) : (
              <p className="text-sm text-[var(--text-primary)] mt-1">{agent.context_mode || '-'}</p>
            )}
          </div>
          {agent.skills && agent.skills.length > 0 && (
            <div>
              <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Skills</label>
              <div className="flex flex-wrap gap-1.5 mt-1">
                {agent.skills.map(s => (
                  <span key={skillLabel(s)} className="text-xs px-2 py-0.5 bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded">{skillLabel(s)}</span>
                ))}
              </div>
            </div>
          )}
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Hosted AgentCard</label>
            {editing ? (
              <textarea
                value={form.agent_card}
                onChange={e => setForm(f => ({ ...f, agent_card: e.target.value }))}
                rows={14}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-xs font-mono text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
              />
            ) : (
              <pre className="mt-1 max-h-80 overflow-auto text-xs bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded-md p-3">{formatAgentCard(agent)}</pre>
            )}
          </div>
          {agent.registered_at && (
            <div>
              <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Registered</label>
              <p className="text-sm text-[var(--text-primary)] mt-1">{new Date(agent.registered_at).toLocaleString()}</p>
            </div>
          )}
          {agent.last_seen && (
            <div>
              <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Last Seen</label>
              <p className="text-sm text-[var(--text-primary)] mt-1">{new Date(agent.last_seen).toLocaleString()}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
