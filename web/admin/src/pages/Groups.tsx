import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Plus, Users, Archive, GitBranch } from 'lucide-react'
import { api, Group } from '../api/client'
import { safeStorage } from '../utils/storage'

type GroupStatusFilter = 'active' | 'archived' | ''

const modes = [
  { value: 'p2p', label: 'P2P' },
  { value: 'leader_led', label: 'Leader-led' },
  { value: 'free_chat', label: 'Free chat' },
  { value: 'roundtable', label: 'Roundtable' },
  { value: 'stateflow', label: 'Stateflow' },
  { value: 'research_long_horizon', label: 'Research' },
]

function modeLabel(mode: string) {
  return modes.find(item => item.value === mode)?.label || mode
}

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function parseJsonField(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  return JSON.parse(trimmed)
}

function defaultRulesForMode(mode: string) {
  switch (mode) {
    case 'p2p':
      return '{\n  "p2p_only": true,\n  "auto_managed": false\n}'
    case 'free_chat':
      return '{\n  "max_speakers": 3,\n  "max_rounds": 1,\n  "auto_artifact": true,\n  "artifact_name": "group-discussion.md"\n}'
    case 'stateflow':
      return '{\n  "workflow": {\n    "type": "manual",\n    "steps": []\n  }\n}'
    case 'roundtable':
      return '{\n  "workflow": {\n    "type": "manual",\n    "steps": []\n  },\n  "required_votes": 2\n}'
    case 'research_long_horizon':
      return '{\n  "workflow": {\n    "type": "manual",\n    "steps": []\n  },\n  "checkpoint_interval": "manual"\n}'
    default:
      return '{\n  "max_rounds": 6\n}'
  }
}

function defaultMemoryForMode(mode: string) {
  if (mode === 'p2p') {
    return '{\n  "hot_messages": 0,\n  "summary": false\n}'
  }
  return '{\n  "hot_messages": 20,\n  "summary": true\n}'
}

export default function Groups() {
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState<GroupStatusFilter>('active')
  const [showCreate, setShowCreate] = useState(false)
  const [error, setError] = useState('')
  const [token, setToken] = useState(() => safeStorage.getItem('admin_token'))
  const [form, setForm] = useState({
    name: '',
    description: '',
    orchestration_mode: 'leader_led',
    rules: defaultRulesForMode('leader_led'),
    memory_policy: defaultMemoryForMode('leader_led'),
  })

  const load = () => {
    setLoading(true)
    api.listGroups(statusFilter || undefined)
      .then(data => setGroups(Array.isArray(data) ? data : []))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load groups'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [statusFilter])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!token) {
      setError('Admin token required')
      return
    }
    try {
      await api.createGroup({
        name: form.name,
        description: form.description,
        orchestration_mode: form.orchestration_mode,
        rules: parseJsonField(form.rules),
        memory_policy: parseJsonField(form.memory_policy),
      }, token)
      setShowCreate(false)
      setForm(f => ({ ...f, name: '', description: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed')
    }
  }

  const handleArchive = async (group: Group) => {
    if (!token) {
      setError('Admin token required')
      return
    }
    if (!confirm(`Archive group "${group.name}"?`)) return
    try {
      await api.archiveGroup(group.id, token)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Archive failed')
    }
  }

  return (
    <div className="p-8 max-w-6xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">Groups</h1>
          <p className="text-xs text-[var(--text-tertiary)] mt-0.5">Native orchestration boundaries</p>
        </div>
        <button
          onClick={() => setShowCreate(v => !v)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] transition-colors"
        >
          <Plus size={14} />
          Create
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 rounded-md bg-[var(--error)]/10 border border-[var(--error)]/30 text-sm text-[var(--error)]">
          {error}
          <button onClick={() => setError('')} className="ml-2 underline">dismiss</button>
        </div>
      )}

      <div className="mb-4">
        <label className="text-xs text-[var(--text-tertiary)]">Admin Token</label>
        <input
          type="password"
          value={token}
          onChange={e => { setToken(e.target.value); safeStorage.setItem('admin_token', e.target.value) }}
          placeholder="Enter admin token for mutations"
          className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
        />
      </div>

      <div className="mb-4 inline-flex rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] p-0.5">
        {[
          { value: 'active', label: 'Active' },
          { value: 'archived', label: 'Archived' },
          { value: '', label: 'All' },
        ].map(item => (
          <button
            key={item.label}
            type="button"
            onClick={() => setStatusFilter(item.value as GroupStatusFilter)}
            className={`px-3 py-1.5 text-xs rounded ${statusFilter === item.value ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {showCreate && (
        <form onSubmit={handleCreate} className="mb-6 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-5 space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-[var(--text-tertiary)]">Name</label>
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                required
                className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--text-tertiary)]">Mode</label>
              <select
                value={form.orchestration_mode}
                onChange={e => setForm(f => ({
                  ...f,
                  orchestration_mode: e.target.value,
                  rules: defaultRulesForMode(e.target.value),
                  memory_policy: defaultMemoryForMode(e.target.value),
                }))}
                className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              >
                {modes.map(mode => <option key={mode.value} value={mode.value}>{mode.label}</option>)}
              </select>
            </div>
          </div>
          <div>
            <label className="text-xs text-[var(--text-tertiary)]">Description</label>
            <input
              value={form.description}
              onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
              className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-[var(--text-tertiary)]">Rules JSON</label>
              <textarea
                value={form.rules}
                onChange={e => setForm(f => ({ ...f, rules: e.target.value }))}
                rows={5}
                className="mt-1 w-full px-3 py-2 text-xs font-mono rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--text-tertiary)]">Memory Policy JSON</label>
              <textarea
                value={form.memory_policy}
                onChange={e => setForm(f => ({ ...f, memory_policy: e.target.value }))}
                rows={5}
                className="mt-1 w-full px-3 py-2 text-xs font-mono rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button type="submit" className="px-4 py-2 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]">Create</button>
            <button type="button" onClick={() => setShowCreate(false)} className="px-4 py-2 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">Cancel</button>
          </div>
        </form>
      )}

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading...</div>
      ) : (
        <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
          {groups.length === 0 ? (
            <div className="p-6 text-center text-sm text-[var(--text-tertiary)]">No groups yet</div>
          ) : (
            groups.map(group => (
              <div key={group.id} className="p-4 hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                <div className="flex items-start justify-between gap-4">
                  <Link to={`/groups/${group.id}`} className="min-w-0 flex-1 no-underline">
                    <div className="flex items-center gap-2">
                      <Users size={16} className="text-[var(--accent)] shrink-0" />
                      <span className="text-sm font-medium text-[var(--text-primary)] truncate">{group.name}</span>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)]">{modeLabel(group.orchestration_mode)}</span>
                      <span className={`text-xs px-2 py-0.5 rounded-full ${group.status === 'active' ? 'bg-[var(--success)]/10 text-[var(--success)]' : 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'}`}>
                        {group.status}
                      </span>
                    </div>
                    <div className="mt-1 text-xs text-[var(--text-tertiary)] truncate">{group.description || group.id}</div>
                    <div className="mt-2 flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
                      <GitBranch size={12} />
                      <span>{formatTime(group.updated_at)}</span>
                    </div>
                  </Link>
                  {group.status !== 'archived' && (
                    <button
                      onClick={() => handleArchive(group)}
                      className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--warning)] rounded transition-colors"
                      title="Archive group"
                    >
                      <Archive size={15} />
                    </button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
