import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight, MessageSquare, Plus, Trash2 } from 'lucide-react'
import { api, Agent } from '../api/client'

export default function Agents() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [showRegister, setShowRegister] = useState(false)
  const [form, setForm] = useState({ name: '', url: '', description: '', token: '', simple_mode: true })
  const [error, setError] = useState('')

  const load = () => {
    setLoading(true)
    api.listAgents()
      .then(data => setAgents(Array.isArray(data) ? data : []))
      .catch(() => setError('Failed to load agents'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      await api.registerAgent({ name: form.name, url: form.url, description: form.description, simple_mode: form.simple_mode }, form.token)
      setShowRegister(false)
      setForm({ name: '', url: '', description: '', token: '', simple_mode: true })
      load()
    } catch (err) {
      setError(String(err))
    }
  }

  const handleDelete = async (name: string) => {
    const token = prompt('Enter admin token to delete agent:')
    if (!token) return
    try {
      await api.deleteAgent(name, token)
      load()
    } catch (err) {
      setError(String(err))
    }
  }

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold">Agents</h2>
        <button
          onClick={() => setShowRegister(!showRegister)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)] transition-colors"
        >
          <Plus size={14} />
          Register
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">
          {error}
        </div>
      )}

      {showRegister && (
        <form onSubmit={handleRegister} className="mb-6 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-5 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <input
              placeholder="Agent name"
              value={form.name}
              onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              className="bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
              required
            />
            <input
              placeholder="Agent URL"
              value={form.url}
              onChange={e => setForm(f => ({ ...f, url: e.target.value }))}
              className="bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
              required
            />
          </div>
          <input
            placeholder="Description (optional)"
            value={form.description}
            onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
            className="w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
          />
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={form.simple_mode}
              onChange={e => setForm(f => ({ ...f, simple_mode: e.target.checked }))}
              className="h-4 w-4 accent-[var(--accent)]"
            />
            Join default P2P network
          </label>
          <div className="flex gap-3">
            <input
              placeholder="Admin token"
              type="password"
              value={form.token}
              onChange={e => setForm(f => ({ ...f, token: e.target.value }))}
              className="flex-1 bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] outline-none focus:border-[var(--accent)]"
              required
            />
            <button type="submit" className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)]">
              Submit
            </button>
            <button type="button" onClick={() => setShowRegister(false)} className="px-4 py-2 text-sm bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)]">
              Cancel
            </button>
          </div>
        </form>
      )}

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading...</div>
      ) : (
        <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
          {agents.length === 0 ? (
            <div className="p-6 text-center text-sm text-[var(--text-tertiary)]">No agents registered</div>
          ) : (
            agents.map(agent => (
              <div key={agent.name} className="flex items-center justify-between gap-4 p-4 hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                <Link to={`/agents/${encodeURIComponent(agent.name)}`} className="flex-1 min-w-0 group">
                  <div className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full ${agent.status === 'connected' ? 'bg-[var(--success)]' : 'bg-[var(--text-tertiary)]'}`} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <div className="text-sm font-medium text-[var(--text-primary)] truncate group-hover:text-[var(--accent)] transition-colors">{agent.name}</div>
                        {agent.mode === 'pull' && (
                          <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded-full bg-amber-500/10 text-amber-500 border border-amber-500/20">pull</span>
                        )}
                      </div>
                      <div className="text-xs text-[var(--text-tertiary)] truncate">{agent.description || agent.url}</div>
                    </div>
                    <ChevronRight size={15} className="ml-auto shrink-0 text-[var(--text-tertiary)] opacity-0 group-hover:opacity-100 transition-opacity" />
                  </div>
                </Link>
                <div className="flex items-center gap-2">
                  <span className={`text-xs px-2 py-0.5 rounded-full ${agent.status === 'connected' ? 'bg-[var(--success)]/10 text-[var(--success)]' : 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'}`}>
                    {agent.status || 'unknown'}
                  </span>
                  <Link
                    to={`/chat/${agent.name}`}
                    className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)] transition-colors"
                    title="Chat with agent"
                  >
                    <MessageSquare size={14} />
                    Chat
                  </Link>
                  <button
                    onClick={() => handleDelete(agent.name)}
                    className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--error)] transition-colors rounded"
                    title="Delete agent"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
