import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Check, ChevronRight, Copy, KeyRound, Pencil, RefreshCcw, Trash2, UserRound, X } from 'lucide-react'
import { api, HumanPresence } from '../api/client'

function formatDate(value?: string) {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export default function Humans() {
  const [humans, setHumans] = useState<HumanPresence[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [editingId, setEditingId] = useState('')
  const [form, setForm] = useState({ handle: '', display_name: '' })
  const [saving, setSaving] = useState(false)
  const [deletingId, setDeletingId] = useState('')
  const [issuingId, setIssuingId] = useState('')
  const [issuedTokens, setIssuedTokens] = useState<Record<string, string>>({})
  const [copiedId, setCopiedId] = useState('')

  const onlineCount = useMemo(() => humans.filter(human => human.online).length, [humans])

  const load = (quiet = false) => {
    if (quiet) {
      setRefreshing(true)
    } else {
      setLoading(true)
    }
    setError('')
    api.listHumans()
      .then(data => setHumans(Array.isArray(data) ? data : []))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load humans'))
      .finally(() => {
        setLoading(false)
        setRefreshing(false)
      })
  }

  useEffect(() => {
    load()
    const timer = window.setInterval(() => load(true), 15000)
    return () => window.clearInterval(timer)
  }, [])

  const startEdit = (human: HumanPresence) => {
    setEditingId(human.id)
    setForm({ handle: human.handle, display_name: human.display_name })
    setError('')
  }

  const cancelEdit = () => {
    setEditingId('')
    setForm({ handle: '', display_name: '' })
  }

  const saveHuman = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!editingId) return
    setSaving(true)
    setError('')
    try {
      await api.updateHuman(editingId, {
        handle: form.handle.trim(),
        display_name: form.display_name.trim(),
      })
      cancelEdit()
      load(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update human')
    } finally {
      setSaving(false)
    }
  }

  const deleteHuman = async (human: HumanPresence) => {
    if (!window.confirm(`Delete human "${human.display_name}" (@${human.handle})? This will remove their sessions and group memberships.`)) {
      return
    }
    setDeletingId(human.id)
    setError('')
    try {
      await api.deleteHuman(human.id)
      if (editingId === human.id) {
        cancelEdit()
      }
      load(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete human')
    } finally {
      setDeletingId('')
    }
  }

  const issueToken = async (human: HumanPresence) => {
    setIssuingId(human.id)
    setError('')
    try {
      const issued = await api.issueHumanToken(human.id)
      setIssuedTokens(tokens => ({ ...tokens, [human.id]: issued.session_token }))
      setCopiedId('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to issue human token')
    } finally {
      setIssuingId('')
    }
  }

  const copyToken = async (humanId: string) => {
    const token = issuedTokens[humanId]
    if (!token) return
    await navigator.clipboard.writeText(token)
    setCopiedId(humanId)
  }

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-lg font-semibold">Humans</h2>
          <p className="text-xs text-[var(--text-tertiary)] mt-1">
            {onlineCount} online / {humans.length} total
          </p>
        </div>
        <button
          onClick={() => load(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--bg-secondary)] border border-[var(--border)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] transition-colors"
          title="Refresh humans"
        >
          <RefreshCcw size={14} className={refreshing ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-[var(--text-tertiary)]">Loading...</div>
      ) : (
        <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg divide-y divide-[var(--border)]">
          {humans.length === 0 ? (
            <div className="p-6 text-center text-sm text-[var(--text-tertiary)]">No humans registered</div>
          ) : (
            humans.map(human => (
              <div key={human.id} className="p-4 hover:bg-[var(--bg-tertiary)]/30 transition-colors">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-3">
                      <div className={`w-2 h-2 rounded-full ${human.online ? 'bg-[var(--success)]' : 'bg-[var(--text-tertiary)]'}`} />
                      <div className="h-9 w-9 rounded-md bg-[var(--bg-tertiary)] flex items-center justify-center text-[var(--text-secondary)]">
                        <UserRound size={17} />
                      </div>
                      {editingId === human.id ? (
                        <form onSubmit={saveHuman} className="flex-1 min-w-0 grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2">
                          <input
                            value={form.handle}
                            onChange={event => setForm(value => ({ ...value, handle: event.target.value }))}
                            className="min-w-0 bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-1.5 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
                            placeholder="handle"
                            required
                          />
                          <input
                            value={form.display_name}
                            onChange={event => setForm(value => ({ ...value, display_name: event.target.value }))}
                            className="min-w-0 bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-1.5 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
                            placeholder="display name"
                          />
                          <div className="flex items-center gap-1">
                            <button
                              type="submit"
                              disabled={saving}
                              className="p-1.5 text-white bg-[var(--accent)] rounded-md hover:bg-[var(--accent-hover)] disabled:opacity-60 transition-colors"
                              title="Save human"
                            >
                              <Check size={15} />
                            </button>
                            <button
                              type="button"
                              onClick={cancelEdit}
                              className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] rounded-md transition-colors"
                              title="Cancel"
                            >
                              <X size={15} />
                            </button>
                          </div>
                        </form>
                      ) : (
                        <Link to={`/humans/${human.id}`} className="min-w-0 group flex items-center gap-2">
                          <span className="min-w-0">
                            <span className="block text-sm font-medium text-[var(--text-primary)] truncate group-hover:text-[var(--accent)] transition-colors">
                              {human.display_name}
                              <span className="ml-2 text-xs font-normal text-[var(--text-tertiary)]">@{human.handle}</span>
                            </span>
                            <span className="block text-xs text-[var(--text-tertiary)] truncate">{human.id}</span>
                          </span>
                          <ChevronRight size={15} className="shrink-0 text-[var(--text-tertiary)] opacity-0 group-hover:opacity-100 transition-opacity" />
                        </Link>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className="hidden sm:block text-right">
                      <div className="text-xs text-[var(--text-secondary)]">{human.active_sessions} active session{human.active_sessions === 1 ? '' : 's'}</div>
                      <div className="text-xs text-[var(--text-tertiary)]">Last seen {formatDate(human.last_seen_at)}</div>
                    </div>
                    <span className={`text-xs px-2 py-0.5 rounded-full ${human.online ? 'bg-[var(--success)]/10 text-[var(--success)]' : 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'}`}>
                      {human.status || (human.online ? 'online' : 'offline')}
                    </span>
                    <button
                      onClick={() => issueToken(human)}
                      disabled={issuingId === human.id}
                      className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] rounded-md disabled:opacity-60 transition-colors"
                      title="Issue human token"
                    >
                      <KeyRound size={14} />
                    </button>
                    <button
                      onClick={() => startEdit(human)}
                      className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)] rounded-md transition-colors"
                      title="Edit human"
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      onClick={() => deleteHuman(human)}
                      disabled={deletingId === human.id}
                      className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--error)] hover:bg-[var(--bg-tertiary)] rounded-md disabled:opacity-60 transition-colors"
                      title="Delete human"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
                {issuedTokens[human.id] && (
                  <div className="mt-3 ml-14 rounded-md border border-[var(--border)] bg-[var(--bg-tertiary)] p-3">
                    <div className="flex items-center justify-between gap-3 mb-2">
                      <span className="text-xs uppercase tracking-wider text-[var(--text-tertiary)]">New human token</span>
                      <button
                        onClick={() => copyToken(human.id)}
                        className="flex items-center gap-1.5 px-2 py-1 text-xs bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                      >
                        <Copy size={13} />
                        {copiedId === human.id ? 'Copied' : 'Copy'}
                      </button>
                    </div>
                    <textarea
                      readOnly
                      value={issuedTokens[human.id]}
                      rows={2}
                      className="w-full resize-none bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md px-3 py-2 text-xs font-mono text-[var(--text-primary)]"
                    />
                    <p className="mt-2 text-xs text-[var(--text-tertiary)]">This token is shown once here. Existing human tokens are stored as hashes and cannot be revealed.</p>
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
