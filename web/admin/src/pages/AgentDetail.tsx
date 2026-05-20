import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Trash2, Save } from 'lucide-react'
import { api, Agent } from '../api/client'

export default function AgentDetail() {
  const { name } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const [agent, setAgent] = useState<Agent | null>(null)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ url: '', description: '' })
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!name) return
    api.getAgent(name)
      .then(a => {
        setAgent(a)
        setForm({ url: a.url, description: a.description || '' })
      })
      .catch(() => setError('Agent not found'))
      .finally(() => setLoading(false))
  }, [name])

  const handleSave = async () => {
    const token = prompt('Enter admin token:')
    if (!token || !name) return
    try {
      await api.deleteAgent(name, token)
      await api.registerAgent({ name, url: form.url, description: form.description }, token)
      setEditing(false)
      setAgent(prev => prev ? { ...prev, url: form.url, description: form.description } : null)
    } catch (err) {
      setError(String(err))
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
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">URL</label>
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
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Description</label>
            {editing ? (
              <input
                value={form.description}
                onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
              />
            ) : (
              <p className="text-sm text-[var(--text-primary)] mt-1">{agent.description || '-'}</p>
            )}
          </div>
          {agent.skills && agent.skills.length > 0 && (
            <div>
              <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Skills</label>
              <div className="flex flex-wrap gap-1.5 mt-1">
                {agent.skills.map(s => (
                  <span key={s} className="text-xs px-2 py-0.5 bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded">{s}</span>
                ))}
              </div>
            </div>
          )}
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
