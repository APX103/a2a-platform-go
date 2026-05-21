import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api, BuiltinAgent, CreateBuiltinAgentReq } from '../api/client'
import { Plus, Trash2, X, MessageSquare, Pencil, Copy } from 'lucide-react'

interface FormMode {
  type: 'create' | 'edit' | 'clone'
  agent?: BuiltinAgent
}

export default function BuiltinAgents() {
  const [agents, setAgents] = useState<BuiltinAgent[]>([])
  const [loading, setLoading] = useState(true)
  const [formMode, setFormMode] = useState<FormMode | null>(null)
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

  const handleEdit = (agent: BuiltinAgent) => {
    setFormMode({ type: 'edit', agent })
  }

  const handleClone = (agent: BuiltinAgent) => {
    setFormMode({ type: 'clone', agent })
  }

  const handleFormSuccess = () => {
    setFormMode(null)
    load()
  }

  if (loading) return <div className="p-8 text-[var(--text-secondary)]">Loading...</div>

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">Builtin Agents</h1>
        <button
          onClick={() => setFormMode({ type: 'create' })}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] transition-colors"
        >
          <Plus size={14} />
          Create
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

      {formMode && (
        <AgentForm
          mode={formMode}
          existingAgents={agents}
          token={token}
          onSuccess={handleFormSuccess}
          onError={setError}
          onCancel={() => setFormMode(null)}
        />
      )}

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
                <div className="flex items-center gap-1.5">
                  <Link
                    to={`/chat/${agent.name}`}
                    className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)] transition-colors"
                    title="Chat with agent"
                  >
                    <MessageSquare size={12} />
                    Chat
                  </Link>
                  <button
                    onClick={() => handleEdit(agent)}
                    className="p-1.5 rounded hover:bg-[var(--bg-tertiary)] text-[var(--text-tertiary)] hover:text-[var(--accent)] transition-colors"
                    title="Edit agent"
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    onClick={() => handleClone(agent)}
                    className="p-1.5 rounded hover:bg-[var(--bg-tertiary)] text-[var(--text-tertiary)] hover:text-[var(--accent)] transition-colors"
                    title="Clone agent"
                  >
                    <Copy size={14} />
                  </button>
                  <button
                    onClick={() => handleDelete(agent.name)}
                    className="p-1.5 rounded hover:bg-[var(--bg-tertiary)] text-[var(--text-tertiary)] hover:text-[var(--error)] transition-colors"
                    title="Delete agent"
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

function AgentForm({
  mode,
  existingAgents,
  token,
  onSuccess,
  onError,
  onCancel,
}: {
  mode: FormMode
  existingAgents: BuiltinAgent[]
  token: string
  onSuccess: () => void
  onError: (e: string) => void
  onCancel: () => void
}) {
  const isEdit = mode.type === 'edit'
  const isClone = mode.type === 'clone'

  const initialName = isClone
    ? generateUniqueName(mode.agent!.name, existingAgents)
    : mode.agent?.name || ''

  const [form, setForm] = useState<CreateBuiltinAgentReq>({
    name: initialName,
    provider: mode.agent?.provider || 'openai',
    base_url: mode.agent?.base_url || '',
    api_key: '',
    model: mode.agent?.model || '',
    description: mode.agent?.description || '',
    system_prompt: mode.agent?.system_prompt || '',
    max_tokens: mode.agent?.max_tokens ?? 4096,
    max_tool_rounds: mode.agent?.max_tool_rounds ?? 10,
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
      if (isEdit) {
        await api.updateBuiltinAgent(mode.agent!.name, form, token)
      } else {
        await api.createBuiltinAgent(form, token)
      }
      onSuccess()
    } catch (err: unknown) {
      onError(err instanceof Error ? err.message : (isEdit ? 'Update failed' : 'Create failed'))
    } finally {
      setSubmitting(false)
    }
  }

  const title = isEdit ? 'Edit Agent' : isClone ? 'Clone Agent' : 'Create Agent'
  const submitLabel = submitting
    ? (isEdit ? 'Saving...' : isClone ? 'Cloning...' : 'Creating...')
    : (isEdit ? 'Save Changes' : isClone ? 'Clone Agent' : 'Create Agent')

  return (
    <div className="mb-6 p-4 rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] space-y-3 relative">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-[var(--text-primary)]">{title}</h3>
        <button onClick={onCancel} className="p-1 rounded hover:bg-[var(--bg-tertiary)] text-[var(--text-tertiary)]">
          <X size={14} />
        </button>
      </div>

      <form onSubmit={handleSubmit} className="space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="text-xs text-[var(--text-tertiary)]">Name *</label>
            <input
              type="text"
              value={form.name}
              onChange={e => set('name', e.target.value)}
              placeholder="my-agent"
              disabled={isEdit}
              className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)] disabled:opacity-50 disabled:cursor-not-allowed"
            />
          </div>
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
          <Field label="Base URL" value={form.base_url || ''} onChange={v => set('base_url', v)} placeholder="https://api.openai.com" />
          <Field label="API Key" value={form.api_key || ''} onChange={v => set('api_key', v)} placeholder="sk-..." type="password" />
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
        <div className="flex items-center gap-2">
          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-2 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] disabled:opacity-50 transition-colors"
          >
            {submitLabel}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm rounded-md border border-[var(--border)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] transition-colors"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
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

/** Generate a unique clone name by appending -copy, -copy-2, -copy-3, etc. */
function generateUniqueName(baseName: string, existingAgents: BuiltinAgent[]): string {
  const existingNames = new Set(existingAgents.map(a => a.name))
  const base = baseName + '-copy'
  if (!existingNames.has(base)) return base
  let n = 2
  while (existingNames.has(`${base}-${n}`)) {
    n++
  }
  return `${base}-${n}`
}
