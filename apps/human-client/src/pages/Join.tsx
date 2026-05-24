import { FormEvent, useState } from 'react'
import { Copy, LogIn, MessageSquare, Network, Shield } from 'lucide-react'
import { api, HumanAuthResponse, HumanJoinPayload, HumanSession } from '../api/client'

function toHumanSession(resp: HumanAuthResponse): HumanSession {
  return {
    human_id: resp.human.id,
    handle: resp.human.handle,
    display_name: resp.human.display_name,
    session_token: resp.session_token,
  }
}

function toHumanJoinPayload(resp: HumanAuthResponse): HumanJoinPayload {
  return {
    session: toHumanSession(resp),
    default_group: resp.default_group,
    default_access_token: resp.default_access_token,
  }
}

export default function Join({ onJoin }: { onJoin: (payload: HumanJoinPayload) => void }) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [loginMethod, setLoginMethod] = useState<'name' | 'token'>('name')
  const [handle, setHandle] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [token, setToken] = useState('')
  const [issued, setIssued] = useState<HumanJoinPayload | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setCopied(false)
    setSubmitting(true)
    try {
      if (mode === 'register') {
        const cleanHandle = handle.trim()
        if (!cleanHandle) {
          setError('Name is required')
          return
        }
        const resp = await api.registerHuman({ handle: cleanHandle, display_name: displayName.trim() })
        setIssued(toHumanJoinPayload(resp))
        return
      }
      if (loginMethod === 'name') {
        const cleanHandle = handle.trim()
        if (!cleanHandle) {
          setError('Name is required')
          return
        }
        const resp = await api.loginHuman({ handle: cleanHandle })
        onJoin(toHumanJoinPayload(resp))
        return
      }
      const cleanToken = token.trim()
      if (!cleanToken) {
        setError('Human token is required')
        return
      }
      const resp = await api.loginHuman({ token: cleanToken })
      onJoin(toHumanJoinPayload(resp))
    } catch (err) {
      setError(err instanceof Error ? err.message : mode === 'register' ? 'Registration failed' : 'Login failed')
    } finally {
      setSubmitting(false)
    }
  }

  const copyIssuedToken = async () => {
    if (!issued?.session.session_token) return
    await navigator.clipboard.writeText(issued.session.session_token)
    setCopied(true)
  }

  if (issued) {
    return (
      <main className="join-shell">
        <section className="join-hero">
          <div className="brand-row">
            <div className="brand-mark"><MessageSquare size={22} /></div>
            <span>A2A Human Client</span>
          </div>
          <h1>Your human token has been issued.</h1>
          <p>
            Save this token if you want a stable credential for this human identity. You can also come back later
            by entering the same unique name.
          </p>
        </section>

        <section className="join-panel">
          <label>
            <span>Human token</span>
            <textarea readOnly value={issued.session.session_token} />
          </label>
          <button type="button" onClick={copyIssuedToken}>
            <Copy size={16} />
            {copied ? 'Copied' : 'Copy Token'}
          </button>
          <button type="button" onClick={() => onJoin(issued)}>
            <LogIn size={16} />
            Enter Client
          </button>
        </section>
      </main>
    )
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
          Register a unique name once, then log in later with that name or with a saved token. New humans are
          added to the default group automatically.
        </p>
        <div className="feature-strip">
          <div><Network size={16} /> Group-scoped discovery</div>
          <div><MessageSquare size={16} /> Chat-style events</div>
          <div><Shield size={16} /> BFF-proxied API</div>
        </div>
      </section>

      <form className="join-panel" onSubmit={submit}>
        <div className="mode-tabs">
          <button type="button" className={mode === 'login' ? 'active' : ''} onClick={() => setMode('login')}>Login</button>
          <button type="button" className={mode === 'register' ? 'active' : ''} onClick={() => setMode('register')}>Register</button>
        </div>
        {mode === 'login' && (
          <div className="mode-tabs compact">
            <button type="button" className={loginMethod === 'name' ? 'active' : ''} onClick={() => setLoginMethod('name')}>Name</button>
            <button type="button" className={loginMethod === 'token' ? 'active' : ''} onClick={() => setLoginMethod('token')}>Token</button>
          </div>
        )}
        {mode === 'register' || loginMethod === 'name' ? (
          <>
            <label>
              <span>Name</span>
              <input
                value={handle}
                onChange={e => setHandle(e.target.value)}
                placeholder="alice"
                autoComplete="username"
              />
            </label>
            {mode === 'register' && (
              <label>
                <span>Display name</span>
                <input
                  value={displayName}
                  onChange={e => setDisplayName(e.target.value)}
                  placeholder="Alice"
                  autoComplete="name"
                />
              </label>
            )}
          </>
        ) : (
          <label>
            <span>Human token</span>
            <input
              value={token}
              onChange={e => setToken(e.target.value)}
              placeholder="paste issued token"
              autoComplete="off"
            />
          </label>
        )}
        {error && <div className="error-box">{error}</div>}
        <button type="submit" disabled={submitting}>
          <LogIn size={16} />
          {submitting ? 'Working...' : mode === 'register' ? 'Register Human' : loginMethod === 'name' ? 'Login With Name' : 'Login With Token'}
        </button>
      </form>
    </main>
  )
}
