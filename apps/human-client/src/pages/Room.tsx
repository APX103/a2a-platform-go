import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw, Send } from 'lucide-react'
import { api, loadRoom, RoomSnapshot, Session } from '../api/client'
import MemberList from '../components/MemberList'
import MessageTimeline from '../components/MessageTimeline'
import OrchestrationPanel from '../components/OrchestrationPanel'
import ArtifactPanel from '../components/ArtifactPanel'

function formatDate(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

export default function Room({
  session,
}: {
  session: Session
}) {
  const [snapshot, setSnapshot] = useState<RoomSnapshot | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const timelineRef = useRef<HTMLDivElement | null>(null)

  const refresh = async () => {
    setError('')
    try {
      const next = await loadRoom(session.group_id)
      setSnapshot(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load room')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
    const timer = window.setInterval(refresh, 5000)
    return () => window.clearInterval(timer)
  }, [session.group_id])

  useEffect(() => {
    timelineRef.current?.scrollTo({ top: timelineRef.current.scrollHeight })
  }, [snapshot?.events.length])

  const humans = useMemo(() => snapshot?.members.filter(m => m.actor_type === 'human') || [], [snapshot])
  const agents = useMemo(() => snapshot?.members.filter(m => m.actor_type === 'agent') || [], [snapshot])

  const send = async (event: FormEvent) => {
    event.preventDefault()
    const content = message.trim()
    if (!content) return
    setSending(true)
    setError('')
    try {
      await api.sendMessage(session.group_id, session.client_id, content)
      setMessage('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Send failed')
    } finally {
      setSending(false)
    }
  }

  if (loading && !snapshot) {
    return <div className="loading-screen">Loading room...</div>
  }

  if (!snapshot) {
    return (
      <div className="loading-screen">
        <p>{error || 'Room not found'}</p>
        <button onClick={refresh}>Retry</button>
      </div>
    )
  }

  return (
    <main className="room-shell">
      <aside className="group-roster">
        <section className="group-card">
          <div className="group-status">{snapshot.group.status}</div>
          <h1>{snapshot.group.name}</h1>
          <p>{snapshot.group.description || snapshot.group.id}</p>
          <div className="group-stats">
            <span>{agents.length} agents</span>
            <span>{humans.length} humans</span>
          </div>
          <div className="group-updated">Updated {formatDate(snapshot.group.updated_at)}</div>
        </section>
        <MemberList members={snapshot.members} />
        <OrchestrationPanel group={snapshot.group} orchestration={snapshot.orchestration} />
        <ArtifactPanel artifacts={snapshot.artifacts} />
      </aside>

      <section className="chat-pane">
        <header className="chat-header">
          <div>
            <h2>{snapshot.group.name}</h2>
            <p>{session.client_id} in {session.group_id}</p>
          </div>
          <div className="header-actions">
            <button onClick={refresh} title="Refresh">
              <RefreshCw size={16} />
            </button>
          </div>
        </header>

        {error && <div className="room-error">{error}</div>}

        <div className="timeline-wrap" ref={timelineRef}>
          <MessageTimeline events={snapshot.events} clientId={session.client_id} />
        </div>

        <form className="composer" onSubmit={send}>
          <textarea
            value={message}
            onChange={e => setMessage(e.target.value)}
            placeholder="Message the group"
            rows={2}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                e.currentTarget.form?.requestSubmit()
              }
            }}
          />
          <button disabled={sending || !message.trim()} title="Send">
            <Send size={18} />
          </button>
        </form>
      </section>
    </main>
  )
}
