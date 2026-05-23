import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Bot, FileText, GitBranch, RefreshCw, Send, UserPlus, Users } from 'lucide-react'
import {
  Agent,
  api,
  Group,
  GroupArtifact,
  GroupEvent,
  GroupInvite,
  GroupMember,
  GroupOrchestrationState,
} from '../api/client'

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function shortId(value: string) {
  return value.length > 12 ? `${value.slice(0, 12)}...` : value
}

function tryFormatJson(value?: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function modeLabel(mode: string) {
  switch (mode) {
    case 'leader_led': return 'Leader-led'
    case 'roundtable': return 'Roundtable'
    case 'stateflow': return 'Stateflow'
    case 'research_long_horizon': return 'Research'
    default: return mode
  }
}

function actorColor(type: string) {
  switch (type) {
    case 'agent': return 'bg-[var(--info)]/10 text-[var(--info)]'
    case 'human': return 'bg-[var(--success)]/10 text-[var(--success)]'
    default: return 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'
  }
}

function actorInitial(id: string) {
  const trimmed = id.trim()
  return (trimmed[0] || '?').toUpperCase()
}

export default function GroupDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [group, setGroup] = useState<Group | null>(null)
  const [members, setMembers] = useState<GroupMember[]>([])
  const [events, setEvents] = useState<GroupEvent[]>([])
  const [artifacts, setArtifacts] = useState<GroupArtifact[]>([])
  const [invites, setInvites] = useState<GroupInvite[]>([])
  const [orchestration, setOrchestration] = useState<GroupOrchestrationState | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [token, setToken] = useState(() => localStorage.getItem('admin_token') || '')
  const [memberForm, setMemberForm] = useState({ actor_id: '', role: 'member' })
  const [joinForm, setJoinForm] = useState({ client_id: 'human-local' })
  const [eventForm, setEventForm] = useState({ sender_type: 'human', sender_id: 'human-local', content: '' })
  const [artifactForm, setArtifactForm] = useState({ name: 'proposal.md', content: '', created_by: 'human-local' })
  const [inviteForm, setInviteForm] = useState({ actor_type_allowed: 'human', role: 'member', max_uses: 20 })
  const [newInviteToken, setNewInviteToken] = useState('')
  const [sendingEvent, setSendingEvent] = useState(false)
  const chatEndRef = useRef<HTMLDivElement | null>(null)

  const load = async () => {
    if (!id) return
    setLoading(true)
    setError('')
    try {
      const [g, m, e, a, o, inviteList, agentList] = await Promise.all([
        api.getGroup(id),
        api.listGroupMembers(id),
        api.listGroupEvents(id, 80),
        api.listGroupArtifacts(id),
        api.getGroupOrchestration(id),
        api.listGroupInvites(id).catch(() => []),
        api.listAgents().catch(() => []),
      ])
      setGroup(g)
      setMembers(Array.isArray(m) ? m : [])
      setEvents(Array.isArray(e) ? e : [])
      setArtifacts(Array.isArray(a) ? a : [])
      setOrchestration(o)
      setInvites(Array.isArray(inviteList) ? inviteList : [])
      setAgents(Array.isArray(agentList) ? agentList : [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load group')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [id])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ block: 'end' })
  }, [events.length])

  const agentOptions = useMemo(() => {
    const memberAgents = new Set(members.filter(m => m.actor_type === 'agent').map(m => m.actor_id))
    return agents.filter(agent => !memberAgents.has(agent.name))
  }, [agents, members])

  const handleAddAgent = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!token) {
      setError('Admin token required')
      return
    }
    if (!memberForm.actor_id) {
      setError('Agent is required')
      return
    }
    try {
      await api.addGroupMember(id, {
        actor_type: 'agent',
        actor_id: memberForm.actor_id,
        role: memberForm.role,
      }, token)
      setMemberForm({ actor_id: '', role: 'member' })
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Add member failed')
    }
  }

  const handleJoin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!joinForm.client_id.trim()) {
      setError('Client id is required')
      return
    }
    try {
      await api.joinGroup(id, { client_id: joinForm.client_id.trim(), capabilities: { ui: 'admin' } }, token)
      setEventForm(f => ({ ...f, sender_type: 'human', sender_id: joinForm.client_id.trim() }))
      setArtifactForm(f => ({ ...f, created_by: joinForm.client_id.trim() }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Join failed')
    }
  }

  const handleCreateInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!token) {
      setError('Admin token required')
      return
    }
    try {
      const invite = await api.createGroupInvite(id, {
        actor_type_allowed: inviteForm.actor_type_allowed || undefined,
        role: inviteForm.role,
        max_uses: inviteForm.max_uses,
      }, token)
      setNewInviteToken(invite.token || '')
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create invite failed')
    }
  }

  const handleSendEvent = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!eventForm.sender_id || !eventForm.content.trim()) {
      setError('Sender and content are required')
      return
    }
    setSendingEvent(true)
    try {
      await api.appendGroupEvent(id, {
        event_type: 'message',
        sender_type: eventForm.sender_type,
        sender_id: eventForm.sender_id,
        content: eventForm.content,
      })
      setEventForm(f => ({ ...f, content: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Send failed')
    } finally {
      setSendingEvent(false)
    }
  }

  const handleEventKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      e.currentTarget.form?.requestSubmit()
    }
  }

  const handleCreateArtifact = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!artifactForm.name || !artifactForm.content.trim()) {
      setError('Artifact name and content are required')
      return
    }
    try {
      await api.createGroupArtifact(id, {
        name: artifactForm.name,
        artifact_type: 'document',
        content: artifactForm.content,
        created_by: artifactForm.created_by,
      })
      setArtifactForm(f => ({ ...f, content: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create artifact failed')
    }
  }

  if (loading) return <div className="p-8 text-sm text-[var(--text-tertiary)]">Loading...</div>
  if (!group) return <div className="p-8 text-sm text-[var(--error)]">Group not found</div>

  return (
    <div className="p-8 max-w-7xl">
      <div className="flex items-center justify-between mb-6">
        <button onClick={() => navigate('/groups')} className="flex items-center gap-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors">
          <ArrowLeft size={14} />
          Back to Groups
        </button>
        <button onClick={load} className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
          <RefreshCw size={14} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 rounded-md bg-[var(--error)]/10 border border-[var(--error)]/30 text-sm text-[var(--error)]">
          {error}
          <button onClick={() => setError('')} className="ml-2 underline">dismiss</button>
        </div>
      )}

      <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-5 mb-6">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2 mb-1">
              <h1 className="text-xl font-semibold text-[var(--text-primary)] truncate">{group.name}</h1>
              <span className="text-xs px-2 py-0.5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)]">{modeLabel(group.orchestration_mode)}</span>
              <span className={`text-xs px-2 py-0.5 rounded-full ${group.status === 'active' ? 'bg-[var(--success)]/10 text-[var(--success)]' : 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]'}`}>{group.status}</span>
            </div>
            <p className="text-sm text-[var(--text-secondary)]">{group.description || shortId(group.id)}</p>
            <p className="mt-2 text-xs text-[var(--text-tertiary)] font-mono">{group.id}</p>
          </div>
          <div className="text-right text-xs text-[var(--text-tertiary)] shrink-0">
            <div>Created {formatTime(group.created_at)}</div>
            <div>Updated {formatTime(group.updated_at)}</div>
          </div>
        </div>
      </section>

      <div className="grid grid-cols-[minmax(260px,320px)_1fr] gap-6">
        <aside className="space-y-6">
          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
            <div className="flex items-center gap-2 mb-3">
              <GitBranch size={15} className="text-[var(--accent)]" />
              <h2 className="text-sm font-medium text-[var(--text-primary)]">Orchestration</h2>
            </div>
            {orchestration && (
              <div className="space-y-3 text-sm">
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Next Action</div>
                  <div className="mt-1 text-[var(--text-primary)]">{orchestration.next_action}</div>
                </div>
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Eligible Speakers</div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {(orchestration.eligible_speakers || []).length === 0 ? (
                      <span className="text-[var(--text-tertiary)]">none</span>
                    ) : orchestration.eligible_speakers.map(speaker => (
                      <span key={speaker} className="text-xs px-2 py-0.5 rounded-full bg-[var(--info)]/10 text-[var(--info)]">{speaker}</span>
                    ))}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Context Policy</div>
                  <p className="mt-1 text-xs text-[var(--text-secondary)] leading-relaxed">{orchestration.context_policy}</p>
                </div>
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] uppercase tracking-wider">Termination</div>
                  <p className="mt-1 text-xs text-[var(--text-secondary)] leading-relaxed">{orchestration.termination_policy}</p>
                </div>
              </div>
            )}
          </section>

          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
            <div className="flex items-center gap-2 mb-3">
              <Users size={15} className="text-[var(--accent)]" />
              <h2 className="text-sm font-medium text-[var(--text-primary)]">Members</h2>
            </div>
            <div className="space-y-2 mb-4">
              {members.length === 0 ? (
                <div className="text-sm text-[var(--text-tertiary)]">No members</div>
              ) : members.map(member => (
                <div key={`${member.actor_type}-${member.actor_id}`} className="flex items-center justify-between gap-2 text-sm">
                  <div className="min-w-0">
                    <div className="text-[var(--text-primary)] truncate">{member.actor_id}</div>
                    <div className="text-xs text-[var(--text-tertiary)]">{member.role}</div>
                  </div>
                  <span className={`text-xs px-2 py-0.5 rounded-full ${actorColor(member.actor_type)}`}>{member.actor_type}</span>
                </div>
              ))}
            </div>

            <div className="mb-3">
              <label className="text-xs text-[var(--text-tertiary)]">Admin Token</label>
              <input
                type="password"
                value={token}
                onChange={e => { setToken(e.target.value); localStorage.setItem('admin_token', e.target.value) }}
                className="mt-1 w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
            </div>

            <form onSubmit={handleAddAgent} className="space-y-2">
              <div className="grid grid-cols-[1fr_92px] gap-2">
                <select
                  value={memberForm.actor_id}
                  onChange={e => setMemberForm(f => ({ ...f, actor_id: e.target.value }))}
                  className="px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)] min-w-0"
                >
                  <option value="">Agent</option>
                  {agentOptions.map(agent => <option key={agent.name} value={agent.name}>{agent.name}</option>)}
                </select>
                <select
                  value={memberForm.role}
                  onChange={e => setMemberForm(f => ({ ...f, role: e.target.value }))}
                  className="px-2 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="member">member</option>
                  <option value="leader">leader</option>
                  <option value="reviewer">reviewer</option>
                  <option value="observer">observer</option>
                </select>
              </div>
              <button className="flex items-center justify-center gap-1.5 w-full px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]">
                <Bot size={14} />
                Add Agent
              </button>
            </form>

            <form onSubmit={handleJoin} className="mt-4 space-y-2">
              <input
                value={joinForm.client_id}
                onChange={e => setJoinForm({ client_id: e.target.value })}
                className="w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
              <button className="flex items-center justify-center gap-1.5 w-full px-3 py-1.5 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
                <UserPlus size={14} />
                Join as Human
              </button>
            </form>
          </section>

          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
            <div className="flex items-center gap-2 mb-3">
              <UserPlus size={15} className="text-[var(--accent)]" />
              <h2 className="text-sm font-medium text-[var(--text-primary)]">Invites</h2>
            </div>
            {newInviteToken && (
              <div className="mb-3 p-2 rounded-md bg-[var(--success)]/10 border border-[var(--success)]/25">
                <div className="text-xs text-[var(--text-tertiary)] mb-1">New token</div>
                <code className="block text-xs break-all text-[var(--text-primary)]">{newInviteToken}</code>
              </div>
            )}
            <form onSubmit={handleCreateInvite} className="space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <select
                  value={inviteForm.actor_type_allowed}
                  onChange={e => setInviteForm(f => ({ ...f, actor_type_allowed: e.target.value }))}
                  className="px-2 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="">any</option>
                  <option value="human">human</option>
                  <option value="agent">agent</option>
                </select>
                <select
                  value={inviteForm.role}
                  onChange={e => setInviteForm(f => ({ ...f, role: e.target.value }))}
                  className="px-2 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="member">member</option>
                  <option value="reviewer">reviewer</option>
                  <option value="observer">observer</option>
                </select>
              </div>
              <input
                type="number"
                min={1}
                value={inviteForm.max_uses}
                onChange={e => setInviteForm(f => ({ ...f, max_uses: Number(e.target.value) || 1 }))}
                className="w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
              <button className="flex items-center justify-center gap-1.5 w-full px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]">
                Generate Invite
              </button>
            </form>
            <div className="mt-3 space-y-1">
              {invites.length === 0 ? (
                <div className="text-xs text-[var(--text-tertiary)]">No invites</div>
              ) : invites.slice(0, 5).map(invite => (
                <div key={invite.id} className="flex items-center justify-between gap-2 text-xs text-[var(--text-tertiary)]">
                  <span>{invite.actor_type_allowed || 'any'} / {invite.role}</span>
                  <span>{invite.used_count}/{invite.max_uses}</span>
                </div>
              ))}
            </div>
          </section>
        </aside>

        <div className="space-y-6 min-w-0">
          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
            <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-[var(--border)] bg-[var(--bg-primary)]">
              <div className="min-w-0">
                <h2 className="text-sm font-medium text-[var(--text-primary)]">Group Chat</h2>
                <div className="mt-0.5 text-xs text-[var(--text-tertiary)] truncate">
                  {group.orchestration_mode === 'leader_led'
                    ? 'Leader-led: messages trigger the leader and create task/trace records.'
                    : 'Event-only for now: automatic agent orchestration is not enabled for this mode.'}
                </div>
              </div>
              <span className="shrink-0 text-xs px-2 py-1 rounded-full bg-[var(--bg-tertiary)] text-[var(--text-secondary)]">{events.length} messages</span>
            </div>
            <div className="h-[min(42vh,520px)] min-h-[260px] overflow-y-auto overflow-x-hidden bg-[var(--bg-primary)] px-5 py-5">
              {events.length === 0 ? (
                <div className="flex h-full items-center justify-center text-sm text-[var(--text-tertiary)]">No messages yet</div>
              ) : (
                <div className="flex min-h-full flex-col justify-end">
                  {events.map(event => (
                    event.sender_type === 'system' || event.event_type === 'orchestration_error' ? (
                      <div key={event.id || `${event.sender_id}-${event.created_at}`} className="my-4 flex justify-center">
                        <div className="max-w-[80%] rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-center">
                          <div className="text-xs text-[var(--text-tertiary)]">{event.sender_id} · {formatTime(event.created_at)}</div>
                          <div className="mt-1 whitespace-pre-wrap break-words text-sm text-[var(--text-secondary)]">{event.content}</div>
                        </div>
                      </div>
                    ) : (
                      <div
                        key={event.id || `${event.sender_id}-${event.created_at}`}
                        className={`mb-5 flex min-w-0 items-end gap-3 ${event.sender_type === 'human' ? 'justify-end' : 'justify-start'}`}
                      >
                        {event.sender_type !== 'human' && (
                          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--info)]/15 text-xs font-medium text-[var(--info)]">
                            {actorInitial(event.sender_id)}
                          </div>
                        )}
                        <div className={`min-w-0 max-w-[min(720px,78%)] ${event.sender_type === 'human' ? 'items-end' : 'items-start'} flex flex-col`}>
                          <div className={`mb-1 flex max-w-full items-center gap-2 px-1 text-xs text-[var(--text-tertiary)] ${event.sender_type === 'human' ? 'flex-row-reverse' : ''}`}>
                            <span className="font-medium text-[var(--text-secondary)] truncate">{event.sender_id}</span>
                            <span className={`px-1.5 py-0.5 rounded-full ${actorColor(event.sender_type)}`}>{event.sender_type}</span>
                            <span className="shrink-0">{formatTime(event.created_at)}</span>
                          </div>
                          <div
                            className={`max-w-full rounded-2xl px-4 py-3 text-sm leading-relaxed shadow-sm whitespace-pre-wrap break-words ${
                              event.sender_type === 'human'
                                ? 'rounded-br-md bg-[var(--accent)] text-white'
                                : 'rounded-bl-md border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                            }`}
                          >
                            {event.content}
                          </div>
                        </div>
                        {event.sender_type === 'human' && (
                          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--accent)]/15 text-xs font-medium text-[var(--accent)]">
                            {actorInitial(event.sender_id)}
                          </div>
                        )}
                      </div>
                    )
                  ))}
                  <div ref={chatEndRef} />
                </div>
              )}
            </div>
            <form onSubmit={handleSendEvent} className="border-t border-[var(--border)] bg-[var(--bg-secondary)] p-4">
              <div className="mb-3 flex flex-wrap items-center gap-2">
                <select
                  value={eventForm.sender_type}
                  onChange={e => setEventForm(f => ({ ...f, sender_type: e.target.value }))}
                  className="h-9 rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 text-sm text-[var(--text-primary)]"
                >
                  <option value="human">human</option>
                  <option value="agent">agent</option>
                  <option value="system">system</option>
                </select>
                <input
                  value={eventForm.sender_id}
                  onChange={e => setEventForm(f => ({ ...f, sender_id: e.target.value }))}
                  className="h-9 min-w-[180px] flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 text-sm text-[var(--text-primary)]"
                />
              </div>
              <div className="flex gap-2">
                <textarea
                  value={eventForm.content}
                  onChange={e => setEventForm(f => ({ ...f, content: e.target.value }))}
                  onKeyDown={handleEventKeyDown}
                  rows={1}
                  placeholder={`Message ${group.name}...`}
                  className="min-h-[52px] flex-1 resize-none rounded-xl border border-[var(--border)] bg-[var(--bg-primary)] px-4 py-3 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--accent)]"
                />
                <button
                  disabled={sendingEvent}
                  className="flex h-[52px] w-[52px] shrink-0 items-center justify-center rounded-xl bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)] disabled:cursor-not-allowed disabled:opacity-50"
                  title="Send message"
                >
                  {sendingEvent ? <RefreshCw size={16} className="animate-spin" /> : <Send size={16} />}
                </button>
              </div>
            </form>
          </section>

          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg">
            <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
              <div className="flex items-center gap-2">
                <FileText size={15} className="text-[var(--accent)]" />
                <h2 className="text-sm font-medium text-[var(--text-primary)]">Artifacts</h2>
              </div>
              <span className="text-xs text-[var(--text-tertiary)]">{artifacts.length}</span>
            </div>
            <div className="grid grid-cols-2 gap-4 p-4">
              <div className="space-y-3">
                {artifacts.length === 0 ? (
                  <div className="text-sm text-[var(--text-tertiary)]">No artifacts</div>
                ) : artifacts.map(artifact => (
                  <div key={artifact.id} className="border border-[var(--border)] rounded-md p-3">
                    <div className="flex items-center justify-between gap-2 mb-1">
                      <div className="text-sm font-medium text-[var(--text-primary)] truncate">{artifact.name}</div>
                      <span className="text-xs text-[var(--text-tertiary)]">v{artifact.version}</span>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-[var(--text-tertiary)] mb-2">
                      <span>{artifact.status}</span>
                      {artifact.created_by && <span>by {artifact.created_by}</span>}
                    </div>
                    <pre className="max-h-36 overflow-auto whitespace-pre-wrap text-xs text-[var(--text-secondary)] bg-[var(--bg-tertiary)] rounded p-2">
                      {artifact.content}
                    </pre>
                  </div>
                ))}
              </div>
              <form onSubmit={handleCreateArtifact} className="space-y-3">
                <div className="grid grid-cols-2 gap-2">
                  <input
                    value={artifactForm.name}
                    onChange={e => setArtifactForm(f => ({ ...f, name: e.target.value }))}
                    className="px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                  />
                  <input
                    value={artifactForm.created_by}
                    onChange={e => setArtifactForm(f => ({ ...f, created_by: e.target.value }))}
                    className="px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                  />
                </div>
                <textarea
                  value={artifactForm.content}
                  onChange={e => setArtifactForm(f => ({ ...f, content: e.target.value }))}
                  rows={11}
                  className="w-full px-3 py-2 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)] resize-none font-mono"
                />
                <button className="w-full px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]">Create Artifact</button>
              </form>
            </div>
          </section>

          <section className="grid grid-cols-2 gap-4">
            <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
              <h2 className="text-sm font-medium text-[var(--text-primary)] mb-2">Rules</h2>
              <pre className="max-h-56 overflow-auto whitespace-pre-wrap text-xs text-[var(--text-secondary)] bg-[var(--bg-tertiary)] rounded p-3">
                {tryFormatJson(group.rules_json) || '{}'}
              </pre>
            </div>
            <div className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
              <h2 className="text-sm font-medium text-[var(--text-primary)] mb-2">Memory Policy</h2>
              <pre className="max-h-56 overflow-auto whitespace-pre-wrap text-xs text-[var(--text-secondary)] bg-[var(--bg-tertiary)] rounded p-3">
                {tryFormatJson(group.memory_policy_json) || '{}'}
              </pre>
            </div>
          </section>

          <div className="text-xs text-[var(--text-tertiary)]">
            <Link to="/groups" className="text-[var(--accent)] hover:text-[var(--accent-hover)] no-underline">Groups</Link>
            <span className="mx-1">/</span>
            <span>{group.name}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
