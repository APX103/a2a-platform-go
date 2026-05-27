import { FormEvent, ReactNode, useEffect, useMemo, useState } from 'react'
import { ArrowRight, KeyRound, LockKeyhole, ShieldCheck } from 'lucide-react'
import { api } from '../api/client'
import { safeStorage } from '../utils/storage'

const ADMIN_TOKEN_KEY = 'admin_token'

function clearLoginThrottle() {
  // no-op: retry throttling removed
}

export default function AdminAuthGate({ children }: { children: ReactNode }) {
  const [checking, setChecking] = useState(true)
  const [authenticated, setAuthenticated] = useState(false)
  const [token, setToken] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')


  useEffect(() => {
    const savedToken = safeStorage.getItem(ADMIN_TOKEN_KEY)
    if (!savedToken) {
      setChecking(false)
      return
    }

    let cancelled = false
    api.validateAdminToken(savedToken)
      .then(() => {
        if (cancelled) return
        setAuthenticated(true)
      })
      .catch(() => {
        if (cancelled) return
        safeStorage.removeItem(ADMIN_TOKEN_KEY)
        setError('Saved admin token is no longer valid.')
      })
      .finally(() => {
        if (!cancelled) setChecking(false)
      })

    return () => {
      cancelled = true
    }
  }, [])

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (submitting) return

    const trimmedToken = token.trim()
    if (!trimmedToken) {
      setError('Admin token is required.')
      return
    }

    setSubmitting(true)
    setError('')
    try {
      await api.validateAdminToken(trimmedToken)
      safeStorage.setItem(ADMIN_TOKEN_KEY, trimmedToken)
      setAuthenticated(true)
    } catch {
      setToken('')
      setError('Invalid admin token.')
    } finally {
      setSubmitting(false)
    }
  }

  if (checking) {
    return (
      <main className="min-h-screen bg-[var(--bg-primary)] flex items-center justify-center px-6">
        <div className="text-sm text-[var(--text-tertiary)]">Checking admin session...</div>
      </main>
    )
  }

  if (authenticated) {
    return <>{children}</>
  }

  return (
    <main className="min-h-screen bg-[var(--bg-primary)] grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_440px]">
      <section className="hidden lg:flex flex-col justify-center px-14 xl:px-20">
        <div className="max-w-2xl">
          <div className="h-12 w-12 rounded-md bg-[var(--text-primary)] text-[var(--bg-primary)] flex items-center justify-center mb-8">
            <ShieldCheck size={24} />
          </div>
          <h1 className="text-5xl font-semibold leading-tight text-[var(--text-primary)]">A2A Platform Admin</h1>
          <p className="mt-5 text-lg leading-8 text-[var(--text-secondary)]">
            Enter the platform admin token before managing agents, humans, groups, tasks, and traces.
          </p>
        </div>
      </section>

      <section className="flex items-center justify-center px-6 py-10">
        <form onSubmit={handleSubmit} className="w-full max-w-md rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] p-6 shadow-sm">
          <div className="flex items-center gap-3 mb-6">
            <div className="h-10 w-10 rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] flex items-center justify-center">
              <LockKeyhole size={20} />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-[var(--text-primary)]">Admin Login</h2>
              <p className="text-xs text-[var(--text-tertiary)]">Protected console access</p>
            </div>
          </div>

          <label className="block text-xs uppercase tracking-wider text-[var(--text-tertiary)] mb-2" htmlFor="admin-token">
            Admin token
          </label>
          <div className="relative">
            <KeyRound size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" />
            <input
              id="admin-token"
              type="password"
              value={token}
              onChange={event => setToken(event.target.value)}
              disabled={submitting}
              autoFocus
              className="w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md pl-9 pr-3 py-2.5 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)] disabled:opacity-60"
              placeholder="Enter admin token"
            />
          </div>

          <div className="min-h-10 mt-3">
            {error && <p className="text-sm text-[var(--error)]">{error}</p>}
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="mt-2 flex w-full items-center justify-center gap-2 rounded-md bg-[var(--accent)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[var(--accent-hover)] disabled:opacity-60"
          >
            {submitting ? 'Checking...' : 'Enter Console'}
            <ArrowRight size={16} />
          </button>
        </form>
      </section>
    </main>
  )
}
