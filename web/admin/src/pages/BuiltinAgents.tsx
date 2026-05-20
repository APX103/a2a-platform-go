import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api, BuiltinAgent, CreateBuiltinAgentReq } from '../api/client'
import { Plus, Trash2, X, MessageSquare } from 'lucide-react'

export default function BuiltinAgents() {
  const [agents, setAgents] = useState<BuiltinAgent[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [error, setError] = useState('')
  const [token, setToken] = useState(() => localStorage.getItem('admin_token') || '')

  const load = async () => {
    try {
      const data = await api.listBuiltinAgents()
      setAgents(data || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (name: string) => {
    if (!token) { setError('Admin token required'); return }
    if (!confirm(`Delete builtin agent "${name}"?`)) return
    try {
      await api.deleteBuiltinAgent(name, token)
      setAgents(agents.filter(a => a.name !== name))
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  if (loading) return <div className="p-8 text-[var(--text-secondary)]">Loading...</div>

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">Builtin Agents</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] transition-colors"
        >
          {showForm ? <X size={14} /> : <Plus size={14} />}
          {showForm ? 'Cancel' : 'Create'}
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 rounded-md bg-red-50 dark:bg-red-900/20 text-[var(--error)] text-sm">
          {error}
          <button onClick={() => setError('')} className="ml-2 underline">dismiss</button>
        </div>
      )}

      <div className="mb-4">
        <label className="text-xs text-[var(--text-tertiary)]">Admin Token</label>
        <input
          type="password"
          value={token}
          onChange={e => { setToken(e.target.value); localStorage.setItem('admin_token', e.target.value) }}
          placeholder="Enter admin token for mutations"
          className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
        />
      </div>

      {showForm && <CreateForm token={token} onCreated={() => { setShowForm(false); load() }} onError={setError} />}

      {agents.length === 0 ? (
        <p className="text-sm text-[var(--text-tertiary)]">No builtin agents configured.</p>
      ) : (
        <div className="space-y-3">
          {agents.map(agent => (
            <div key={agent.name} className="p-4 rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)]">
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <h3 className="font-medium text-[var(--text-primary)]">{agent.name}</h3>
                  <p className="text-xs text-[var(--text-tertiary)] mt-0.5">
                    {agent.provider} / {agent.model}
                  </p>
                  {agent.description && (
                    <p className="text-sm text-[var(--text-secondary)] mt-1">{agent.description}</p>
                  )}
                </div>
                <div className="flex items-center gap-2">
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
                    className="p-1.5 rounded hover:bg-[var(--bg-tertiary)] text-[var(--text-tertiary)] hover:text-[var(--error)] transition-colors"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
              <div className="mt-2 flex gap-3 text-xs text-[var(--text-tertiary)]">
                <span>Max tokens: {agent.max_tokens}</span>
                <span>Tool rounds: {agent.max_tool_rounds}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function CreateForm({ token, onCreated, onError }: { token: string; onCreated: () => void; onError: (e: string) => void }) {
  const [form, setForm] = useState<CreateBuiltinAgentReq>({
    name: '',
    provider: 'openai',
    base_url: '',
    api_key: '',
    model: '',
    description: '',
    system_prompt: '',
    max_tokens: 4096,
    max_tool_rounds: 10,
  })
  const [submitting, setSubmitting] = useState(false)

  const set = (key: keyof CreateBuiltinAgentReq, value: string | number) =>
    setForm(f => ({ ...f, [key]: value }))

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!token) { onError('Admin token required'); return }
    if (!form.name || !form.model) { onError('Name and model are required'); return }
    setSubmitting(true)
    try {
      await api.createBuiltinAgent(form, token)
      onCreated()
    } catch (err: unknown) {
      onError(err instanceof Error ? err.message : 'Create failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="mb-6 p-4 rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Name *" value={form.name} onChange={v => set('name', v)} placeholder="my-agent" />
        <div>
          <label className="text-xs text-[var(--text-tertiary)]">Provider *</label>
          <select
            value={form.provider}
            onChange={e => set('provider', e.target.value)}
            className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
          >
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
          </select>
        </div>
        <Field label="Base URL" value={form.base_url} onChange={v => set('base_url', v)} placeholder="https://api.openai.com" />
        <Field label="API Key *" value={form.api_key} onChange={v => set('api_key', v)} placeholder="sk-..." type="password" />
        <Field label="Model *" value={form.model} onChange={v => set('model', v)} placeholder="gpt-4o" />
        <Field label="Description" value={form.description || ''} onChange={v => set('description', v)} placeholder="A helpful assistant" />
      </div>
      <div>
        <label className="text-xs text-[var(--text-tertiary)]">System Prompt</label>
        <textarea
          value={form.system_prompt || ''}
          onChange={e => set('system_prompt', e.target.value)}
          rows={3}
          placeholder="You are a helpful assistant."
          className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)] resize-none"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-xs text-[var(--text-tertiary)]">Max Tokens</label>
          <input
            type="number"
            value={form.max_tokens}
            onChange={e => set('max_tokens', Number(e.target.value))}
            className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
          />
        </div>
        <div>
          <label className="text-xs text-[var(--text-tertiary)]">Max Tool Rounds</label>
          <input
            type="number"
            value={form.max_tool_rounds}
            onChange={e => set('max_tool_rounds', Number(e.target.value))}
            className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
          />
        </div>
      </div>
      <button
        type="submit"
        disabled={submitting}
        className="px-4 py-2 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] disabled:opacity-50 transition-colors"
      >
        {submitting ? 'Creating...' : 'Create Agent'}
      </button>
    </form>
  )
}

function Field({ label, value, onChange, placeholder, type = 'text' }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string
}) {
  return (
    <div>
      <label className="text-xs text-[var(--text-tertiary)]">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
      />
    </div>
  )
}
