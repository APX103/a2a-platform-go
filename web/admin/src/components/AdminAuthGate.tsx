import { FormEvent, ReactNode, useEffect, useMemo, useState } from 'react'
import { ArrowRight, KeyRound, LockKeyhole, ShieldCheck } from 'lucide-react'
import { api } from '../api/client'
import { safeStorage } from '../utils/storage'

const ADMIN_TOKEN_KEY = 'admin_token'
const FAILED_ATTEMPTS_KEY = 'admin_auth_failed_attempts'
const LOCKED_UNTIL_KEY = 'admin_auth_locked_until'
const MAX_ATTEMPTS = 3
const LOCK_MS = 60_000

function readNumber(key: string) {
  const value = Number(safeStorage.getItem(key))
  return Number.isFinite(value) ? value : 0
}

function clearLoginThrottle() {
  safeStorage.removeItem(FAILED_ATTEMPTS_KEY)
  safeStorage.removeItem(LOCKED_UNTIL_KEY)
}

export default function AdminAuthGate({ children }: { children: ReactNode }) {
  const [checking, setChecking] = useState(true)
  const [authenticated, setAuthenticated] = useState(false)
  const [token, setToken] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [failedAttempts, setFailedAttempts] = useState(() => readNumber(FAILED_ATTEMPTS_KEY))
  const [lockedUntil, setLockedUntil] = useState(() => readNumber(LOCKED_UNTIL_KEY))
  const [now, setNow] = useState(Date.now())

  const remainingMs = Math.max(0, lockedUntil - now)
  const locked = remainingMs > 0
  const remainingSeconds = Math.ceil(remainingMs / 1000)
  const attemptsLeft = useMemo(() => Math.max(0, MAX_ATTEMPTS - failedAttempts), [failedAttempts])

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
        clearLoginThrottle()
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

  useEffect(() => {
    if (!locked) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [locked])

  useEffect(() => {
    if (locked || lockedUntil === 0) return
    safeStorage.removeItem(LOCKED_UNTIL_KEY)
    setLockedUntil(0)
  }, [locked, lockedUntil])

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (locked || submitting) return

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
      clearLoginThrottle()
      setFailedAttempts(0)
      setLockedUntil(0)
      setAuthenticated(true)
    } catch {
      const nextAttempts = failedAttempts + 1
      safeStorage.setItem(FAILED_ATTEMPTS_KEY, String(nextAttempts))
      setFailedAttempts(nextAttempts)
      setToken('')

      if (nextAttempts >= MAX_ATTEMPTS) {
        const nextLockedUntil = Date.now() + LOCK_MS
        safeStorage.setItem(LOCKED_UNTIL_KEY, String(nextLockedUntil))
        setLockedUntil(nextLockedUntil)
        setNow(Date.now())
        setError('Too many failed attempts. Try again in 1 minute.')
      } else {
        setError(`Invalid admin token. ${MAX_ATTEMPTS - nextAttempts} attempt${MAX_ATTEMPTS - nextAttempts === 1 ? '' : 's'} left.`)
      }
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
              disabled={locked || submitting}
              autoFocus
              className="w-full bg-[var(--bg-tertiary)] border border-[var(--border)] rounded-md pl-9 pr-3 py-2.5 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)] disabled:opacity-60"
              placeholder={locked ? `Locked for ${remainingSeconds}s` : 'Enter admin token'}
            />
          </div>

          <div className="min-h-10 mt-3">
            {error && <p className="text-sm text-[var(--error)]">{error}</p>}
            {!error && !locked && failedAttempts > 0 && (
              <p className="text-sm text-[var(--text-tertiary)]">{attemptsLeft} attempt{attemptsLeft === 1 ? '' : 's'} left.</p>
            )}
            {locked && (
              <p className="text-sm text-[var(--warning)]">Login locked. Try again in {remainingSeconds}s.</p>
            )}
          </div>

          <button
            type="submit"
            disabled={locked || submitting}
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
