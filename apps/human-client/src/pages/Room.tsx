import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw, Send } from 'lucide-react'
import { api, DirectAgentMessage, loadRoom, RoomSnapshot, Session } from '../api/client'
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
  const [activeAgent, setActiveAgent] = useState('')
  const [directMessages, setDirectMessages] = useState<Record<string, DirectAgentMessage[]>>({})
  const timelineRef = useRef<HTMLDivElement | null>(null)

  const refresh = async () => {
    setError('')
    try {
      const next = await loadRoom(session.group_id, session.access_token)
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
  }, [snapshot?.events.length, activeAgent, directMessages])

  const humans = useMemo(() => snapshot?.members.filter(m => m.actor_type === 'human') || [], [snapshot])
  const agents = useMemo(() => snapshot?.members.filter(m => m.actor_type === 'agent') || [], [snapshot])
  const isP2P = snapshot?.group.orchestration_mode === 'p2p'

  const send = async (event: FormEvent) => {
    event.preventDefault()
    const content = message.trim()
    if (!content) return
    setSending(true)
    setError('')
    try {
      if (activeAgent) {
        const humanMessage: DirectAgentMessage = {
          id: `human-${Date.now()}`,
          sender: 'human',
          agent: activeAgent,
          content,
          created_at: new Date().toISOString(),
        }
        setDirectMessages(prev => ({
          ...prev,
          [activeAgent]: [...(prev[activeAgent] || []), humanMessage],
        }))
        setMessage('')
        const agentMessageId = `agent-${Date.now()}`
        const agentMessage: DirectAgentMessage = {
          id: agentMessageId,
          sender: 'agent',
          agent: activeAgent,
          content: '',
          created_at: new Date().toISOString(),
        }
        setDirectMessages(prev => ({
          ...prev,
          [activeAgent]: [...(prev[activeAgent] || []), agentMessage],
        }))
        await api.sendDirectToAgentStreaming(
          activeAgent,
          session.access_token,
          session.human_id,
          content,
          {
            onTextDelta: (text) => {
              setDirectMessages(prev => {
                const msgs = prev[activeAgent] || []
                const idx = msgs.findIndex(m => m.id === agentMessageId)
                if (idx === -1) return prev
                const updated = [...msgs]
                updated[idx] = { ...updated[idx], content: updated[idx].content + text }
                return { ...prev, [activeAgent]: updated }
              })
            },
            onThinkingDelta: (text) => {
              setDirectMessages(prev => {
                const msgs = prev[activeAgent] || []
                const idx = msgs.findIndex(m => m.id === agentMessageId)
                if (idx === -1) return prev
                const updated = [...msgs]
                updated[idx] = { ...updated[idx], reasoning_content: (updated[idx].reasoning_content || '') + text }
                return { ...prev, [activeAgent]: updated }
              })
            },
            onError: (err) => {
              setError(err)
              setSending(false)
            },
            onDone: () => {
              setSending(false)
            },
          },
        )
        return
      }
      if (isP2P) {
        setError('Select an agent from Participants to start a direct P2P chat.')
        return
      }
      await api.sendMessage(session.group_id, session.access_token, session.human_id, content)
      setMessage('')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Send failed')
    } finally {
      setSending(false)
    }
  }

  const activeDirectMessages = activeAgent ? directMessages[activeAgent] || [] : []

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
        <MemberList members={snapshot.members} activeAgent={activeAgent} onSelectAgent={setActiveAgent} />
        <OrchestrationPanel group={snapshot.group} orchestration={snapshot.orchestration} />
        <ArtifactPanel artifacts={snapshot.artifacts} />
      </aside>

      <section className="chat-pane">
        <header className="chat-header">
          <div>
            <h2>{snapshot.group.name}</h2>
            <p>
              {activeAgent
                ? `${session.display_name} (@${session.handle}) -> ${activeAgent}`
                : `${session.display_name} (@${session.handle}) in ${session.group_id}`}
            </p>
          </div>
          <div className="header-actions">
            {activeAgent && (
              <button onClick={() => setActiveAgent('')} title="Back to group">
                Group
              </button>
            )}
            <button onClick={refresh} title="Refresh">
              <RefreshCw size={16} />
            </button>
          </div>
        </header>

        {error && <div className="room-error">{error}</div>}

        <div className="timeline-wrap" ref={timelineRef}>
          {activeAgent ? (
            activeDirectMessages.length === 0 ? (
              <div className="empty-timeline">No direct messages with {activeAgent}</div>
            ) : (
              <div className="timeline">
                {activeDirectMessages.map(item => (
                  <article className={`message ${item.sender === 'human' ? 'mine' : ''}`} key={item.id}>
                    <div className="message-head">
                      <span>{item.sender === 'human' ? session.display_name : item.agent}</span>
                      <b>{item.sender === 'human' ? 'you' : 'agent'}</b>
                      <time>{new Date(item.created_at).toLocaleTimeString()}</time>
                    </div>
                    <div className="message-body">{item.content}</div>
                    {item.reasoning_content && (
                      <div className="message-thinking">
                        <div className="thinking-label">Thinking</div>
                        <div className="thinking-body">{item.reasoning_content}</div>
                      </div>
                    )}
                  </article>
                ))}
              </div>
            )
          ) : (
            <MessageTimeline events={snapshot.events} clientId={session.human_id} />
          )}
        </div>

        <form className="composer" onSubmit={send}>
          <textarea
            value={message}
            onChange={e => setMessage(e.target.value)}
            placeholder={activeAgent ? `Message ${activeAgent}` : isP2P ? 'Select an agent to start a direct chat' : 'Message the group'}
            rows={2}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                e.currentTarget.form?.requestSubmit()
              }
            }}
          />
          <button disabled={sending || !message.trim() || (!activeAgent && isP2P)} title="Send">
            <Send size={18} />
          </button>
        </form>
      </section>
    </main>
  )
}
