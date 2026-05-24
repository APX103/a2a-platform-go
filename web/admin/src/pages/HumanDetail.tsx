import { FormEvent, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Check, Copy, KeyRound, Save, Trash2, UserRound, X } from 'lucide-react'
import { api, HumanPresence, HumanProfile } from '../api/client'

function formatDate(value?: string) {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export default function HumanDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [human, setHuman] = useState<HumanProfile | null>(null)
  const [presence, setPresence] = useState<HumanPresence | null>(null)
  const [form, setForm] = useState({ handle: '', display_name: '' })
  const [editing, setEditing] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [issuing, setIssuing] = useState(false)
  const [issuedToken, setIssuedToken] = useState('')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    if (!id) return
    setLoading(true)
    setError('')
    try {
      const [profile, humans] = await Promise.all([
        api.getHuman(id),
        api.listHumans(),
      ])
      const matchedPresence = humans.find(item => item.id === profile.id) ?? null
      setHuman(profile)
      setPresence(matchedPresence)
      setForm({ handle: profile.handle, display_name: profile.display_name })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load human')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [id])

  const saveHuman = async (event: FormEvent) => {
    event.preventDefault()
    if (!id) return
    setSaving(true)
    setError('')
    try {
      const updated = await api.updateHuman(id, {
        handle: form.handle.trim(),
        display_name: form.display_name.trim(),
      })
      setHuman(updated)
      setPresence(current => current ? { ...current, handle: updated.handle, display_name: updated.display_name } : current)
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update human')
    } finally {
      setSaving(false)
    }
  }

  const deleteHuman = async () => {
    if (!human || !id) return
    if (!window.confirm(`Delete human "${human.display_name}" (@${human.handle})? This will remove their sessions and group memberships.`)) {
      return
    }
    setDeleting(true)
    setError('')
    try {
      await api.deleteHuman(id)
      navigate('/humans')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete human')
      setDeleting(false)
    }
  }

  const issueToken = async () => {
    if (!id) return
    setIssuing(true)
    setCopied(false)
    setError('')
    try {
      const issued = await api.issueHumanToken(id)
      setIssuedToken(issued.session_token)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to issue human token')
    } finally {
      setIssuing(false)
    }
  }

  const copyToken = async () => {
    if (!issuedToken) return
    await navigator.clipboard.writeText(issuedToken)
    setCopied(true)
  }

  if (loading) return <div className="p-8 text-sm text-[var(--text-tertiary)]">Loading...</div>
  if (!human) return <div className="p-8 text-sm text-[var(--error)]">{error || 'Human not found'}</div>

  return (
    <div className="p-8 max-w-3xl">
      <button onClick={() => navigate('/humans')} className="flex items-center gap-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-6 transition-colors">
        <ArrowLeft size={14} />
        Back to Humans
      </button>

      {error && (
        <div className="mb-4 p-3 bg-[var(--error)]/10 border border-[var(--error)]/30 rounded-md text-sm text-[var(--error)]">{error}</div>
      )}

      <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-6">
        <div className="flex items-start justify-between gap-4 mb-6">
          <div className="flex items-center gap-3 min-w-0">
            <div className={`w-3 h-3 rounded-full shrink-0 ${presence?.online ? 'bg-[var(--success)]' : 'bg-[var(--text-tertiary)]'}`} />
            <div className="h-11 w-11 rounded-md bg-[var(--bg-tertiary)] flex items-center justify-center text-[var(--text-secondary)] shrink-0">
              <UserRound size={20} />
            </div>
            <div className="min-w-0">
              <h2 className="text-xl font-semibold truncate">{human.display_name}</h2>
              <p className="text-sm text-[var(--text-tertiary)] truncate">@{human.handle}</p>
            </div>
          </div>
          <div className="flex gap-2">
            {editing ? (
              <>
                <button
                  type="submit"
                  form="human-detail-form"
                  disabled={saving}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--accent)] text-white rounded-md hover:bg-[var(--accent-hover)] disabled:opacity-60"
                >
                  <Save size={14} />
                  Save
                </button>
                <button
                  onClick={() => {
                    setForm({ handle: human.handle, display_name: human.display_name })
                    setEditing(false)
                  }}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)]"
                >
                  <X size={14} />
                  Cancel
                </button>
              </>
            ) : (
              <button onClick={() => setEditing(true)} className="px-3 py-1.5 text-sm bg-[var(--bg-tertiary)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)]">
                Edit
              </button>
            )}
            <button
              onClick={deleteHuman}
              disabled={deleting}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--error)]/10 text-[var(--error)] rounded-md hover:bg-[var(--error)]/20 disabled:opacity-60"
            >
              <Trash2 size={14} />
              Delete
            </button>
          </div>
        </div>

        <div className="mb-6 grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div className="rounded-md border border-[var(--border)] bg-[var(--bg-tertiary)] px-3 py-2">
            <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Status</div>
            <div className={`mt-1 text-sm font-medium ${presence?.online ? 'text-[var(--success)]' : 'text-[var(--text-secondary)]'}`}>
              {presence?.status || (presence?.online ? 'online' : 'offline')}
            </div>
          </div>
          <div className="rounded-md border border-[var(--border)] bg-[var(--bg-tertiary)] px-3 py-2">
            <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Active Sessions</div>
            <div className="mt-1 text-sm font-medium text-[var(--text-primary)]">{presence?.active_sessions ?? 0}</div>
          </div>
          <div className="rounded-md border border-[var(--border)] bg-[var(--bg-tertiary)] px-3 py-2">
            <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Last Seen</div>
            <div className="mt-1 text-sm font-medium text-[var(--text-primary)] truncate">{formatDate(presence?.last_seen_at)}</div>
          </div>
        </div>

        <form id="human-detail-form" onSubmit={saveHuman} className="space-y-4">
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Human ID</label>
            <p className="text-sm text-[var(--text-primary)] mt-1 font-mono break-all">{human.id}</p>
          </div>
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Handle</label>
            {editing ? (
              <input
                value={form.handle}
                onChange={event => setForm(value => ({ ...value, handle: event.target.value }))}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
                required
              />
            ) : (
              <p className="text-sm text-[var(--text-primary)] mt-1">@{human.handle}</p>
            )}
          </div>
          <div>
            <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Display Name</label>
            {editing ? (
              <input
                value={form.display_name}
                onChange={event => setForm(value => ({ ...value, display_name: event.target.value }))}
                className="mt-1 w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
              />
            ) : (
              <p className="text-sm text-[var(--text-primary)] mt-1">{human.display_name}</p>
            )}
          </div>
          {presence?.created_at && (
            <div>
              <label className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Registered</label>
              <p className="text-sm text-[var(--text-primary)] mt-1">{formatDate(presence.created_at)}</p>
            </div>
          )}
        </form>

        <div className="mt-6 rounded-md border border-[var(--border)] bg-[var(--bg-tertiary)] p-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">Human Session Token</h3>
              <p className="text-xs text-[var(--text-tertiary)] mt-1">Existing tokens are stored as hashes and cannot be revealed. Issue a new one when the user needs to recover access.</p>
            </div>
            <button
              onClick={issueToken}
              disabled={issuing}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-[var(--bg-secondary)] border border-[var(--border)] text-[var(--text-secondary)] rounded-md hover:text-[var(--text-primary)] disabled:opacity-60"
            >
              <KeyRound size={14} />
              {issuing ? 'Issuing...' : 'Issue token'}
            </button>
          </div>

          {issuedToken && (
            <div className="mt-4">
              <div className="flex items-center justify-between gap-3 mb-2">
                <span className="text-xs uppercase tracking-wider text-[var(--text-tertiary)]">New human token</span>
                <button
                  onClick={copyToken}
                  className="flex items-center gap-1.5 px-2 py-1 text-xs bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                >
                  {copied ? <Check size={13} /> : <Copy size={13} />}
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
              <textarea
                readOnly
                value={issuedToken}
                rows={2}
                className="w-full resize-none bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md px-3 py-2 text-xs font-mono text-[var(--text-primary)]"
              />
              <p className="mt-2 text-xs text-[var(--text-tertiary)]">This token is shown once here. It will not be recoverable after leaving this page.</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
