import { FormEvent, useState } from 'react'
import { LogIn, MessageSquare, Network, Shield } from 'lucide-react'
import { api } from '../api/client'

export default function Join({ onJoin }: { onJoin: (clientId: string) => void }) {
  const [clientId, setClientId] = useState(() => localStorage.getItem('a2a_human_client_id') || 'human-local')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    const client_id = clientId.trim()
    if (!client_id) {
      setError('Client id is required')
      return
    }
    setSubmitting(true)
    try {
      await api.saveSession(client_id)
      localStorage.setItem('a2a_human_client_id', client_id)
      onJoin(client_id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign in failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="join-shell">
      <section className="join-hero">
        <div className="brand-row">
          <div className="brand-mark"><MessageSquare size={22} /></div>
          <span>A2A Human Client</span>
        </div>
        <h1>Join an agent group as a human participant.</h1>
        <p>
          Pick a client id first. After that, you can join groups by token, inspect the visible
          participants in each group, and talk with agents through group-scoped rooms.
        </p>
        <div className="feature-strip">
          <div><Network size={16} /> Group-scoped discovery</div>
          <div><MessageSquare size={16} /> Chat-style events</div>
          <div><Shield size={16} /> BFF-proxied API</div>
        </div>
      </section>

      <form className="join-panel" onSubmit={submit}>
        <label>
          <span>Client ID</span>
          <input
            value={clientId}
            onChange={e => setClientId(e.target.value)}
            placeholder="human-local"
            autoComplete="username"
          />
        </label>
        {error && <div className="error-box">{error}</div>}
        <button type="submit" disabled={submitting}>
          <LogIn size={16} />
          {submitting ? 'Entering...' : 'Enter Client'}
        </button>
      </form>
    </main>
  )
}
