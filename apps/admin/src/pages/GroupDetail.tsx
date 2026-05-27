import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowDown, ArrowLeft, ArrowUp, Bot, ExternalLink, FileText, GitBranch, KeyRound, ListOrdered, MessageSquare, RefreshCw, Save, Send, Trash2, UserPlus, Users } from 'lucide-react'
import {
  Agent,
  api,
  Group,
  GroupArtifact,
  GroupEvent,
  GroupInvite,
  GroupMember,
  GroupOrchestrationState,
  GroupStreamEvent,
} from '../api/client'
import { safeStorage } from '../utils/storage'

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

type FlowStep = {
  id: string
  name: string
  agent: string
  role: string
  system_prompt: string
}

function parseObjectJson(value?: string): Record<string, unknown> {
  if (!value) return {}
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

function workflowStepsFromRules(value?: string): FlowStep[] {
  const rules = parseObjectJson(value)
  const workflow = rules.workflow && typeof rules.workflow === 'object' && !Array.isArray(rules.workflow)
    ? rules.workflow as Record<string, unknown>
    : {}
  const rawSteps = Array.isArray(workflow.steps)
    ? workflow.steps
    : Array.isArray(rules.steps)
      ? rules.steps
      : Array.isArray(rules.phases)
        ? rules.phases
        : []
  return rawSteps.map((item, index) => {
    const step = item && typeof item === 'object' ? item as Record<string, unknown> : {}
    return {
      id: String(step.id || `step-${index + 1}`),
      name: String(step.name || step.phase || `Step ${index + 1}`),
      agent: String(step.agent || step.actor || ''),
      role: String(step.role || 'worker'),
      system_prompt: String(step.system_prompt || step.prompt || ''),
    }
  })
}

function memoryPolicyForUpdate(value?: string) {
  return Object.keys(parseObjectJson(value)).length > 0 ? parseObjectJson(value) : undefined
}

function isFlowMode(mode: string) {
  return mode === 'roundtable' || mode === 'stateflow' || mode === 'research_long_horizon'
}

function modeLabel(mode: string) {
  switch (mode) {
    case 'p2p': return 'P2P'
    case 'leader_led': return 'Leader-led'
    case 'free_chat': return 'Free chat'
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

function modeHint(mode: string) {
  switch (mode) {
    case 'p2p':
      return 'P2P network: members can discover each other and use direct agent calls, but group chat does not trigger orchestration.'
    case 'leader_led':
      return 'Leader-led: messages trigger the leader and create task/trace records.'
    case 'free_chat':
      return 'Free chat: agents observe each new message and decide whether to reply.'
    case 'roundtable':
      return 'Roundtable: structured review around shared artifacts.'
    case 'stateflow':
      return 'Stateflow: configured phases decide who can speak next.'
    case 'research_long_horizon':
      return 'Research: long-running workstreams, checkpoints, and artifacts.'
    default:
      return 'Custom orchestration mode.'
  }
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
  const [token, setToken] = useState(() => safeStorage.getItem('admin_token'))
  const [memberForm, setMemberForm] = useState({ actor_id: '', role: 'member' })
  const [joinForm, setJoinForm] = useState({ client_id: 'human-local' })
  const [inviteJoinForm, setInviteJoinForm] = useState({ invite_token: '', actor_id: 'human-local' })
  const [memberToken, setMemberToken] = useState('')
  const [eventForm, setEventForm] = useState({ sender_type: 'human', sender_id: 'human-local', content: '' })
  const [artifactForm, setArtifactForm] = useState({ name: 'proposal.md', content: '', created_by: 'human-local' })
  const [inviteForm, setInviteForm] = useState({ actor_type_allowed: 'human', role: 'member', max_uses: 20 })
  const [newInviteToken, setNewInviteToken] = useState('')
  const [sendingEvent, setSendingEvent] = useState(false)
  const [topicForm, setTopicForm] = useState({ entry_agent: '', title: '', content: '' })
  const [topicStarting, setTopicStarting] = useState(false)
  const [flowSteps, setFlowSteps] = useState<FlowStep[]>([])
  const [selectedStep, setSelectedStep] = useState(0)
  const [flowSaving, setFlowSaving] = useState(false)
  const [copiedToken, setCopiedToken] = useState<string | null>(null)
  const chatEndRef = useRef<HTMLDivElement | null>(null)
  const streamingEventIdsRef = useRef<Record<string, number>>({})

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedToken(text)
      setTimeout(() => setCopiedToken(null), 2000)
    } catch {
      // fallback
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setCopiedToken(text)
      setTimeout(() => setCopiedToken(null), 2000)
    }
  }

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
    if (!id) {
      setMemberToken('')
      return
    }
    setMemberToken(safeStorage.getItem(`group_member_token_${id}`))
  }, [id])

  useEffect(() => {
    if (!group || !isFlowMode(group.orchestration_mode)) {
      setFlowSteps([])
      setSelectedStep(0)
      return
    }
    const nextSteps = workflowStepsFromRules(group.rules_json)
    setFlowSteps(nextSteps)
    setSelectedStep(0)
  }, [group?.id, group?.orchestration_mode, group?.rules_json])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ block: 'end' })
  }, [events.length])

  const agentOptions = useMemo(() => {
    const memberAgents = new Set(members.filter(m => m.actor_type === 'agent').map(m => m.actor_id))
    return agents.filter(agent => !memberAgents.has(agent.name))
  }, [agents, members])

  const memberAgentNames = useMemo(
    () => members.filter(member => member.actor_type === 'agent').map(member => member.actor_id),
    [members],
  )

  const topicAgentOptions = useMemo(() => {
    const flowAgentNames = flowSteps.map(step => step.agent).filter(Boolean)
    const preferred = memberAgentNames.length > 0 ? memberAgentNames : flowAgentNames
    return Array.from(new Set(preferred.length > 0 ? preferred : agents.map(agent => agent.name)))
  }, [agents, flowSteps, memberAgentNames])

  useEffect(() => {
    setTopicForm(form => {
      if (form.entry_agent && topicAgentOptions.includes(form.entry_agent)) return form
      return { ...form, entry_agent: topicAgentOptions[0] || '' }
    })
  }, [topicAgentOptions])

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

  const handleRemoveMember = async (member: GroupMember) => {
    if (!id) return
    if (!token) {
      setError('Admin token required')
      return
    }
    try {
      const updated = await api.removeGroupMember(id, member.actor_type, member.actor_id, token)
      setMembers(Array.isArray(updated) ? updated : [])
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Remove member failed')
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

  const handleJoinByInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    if (!inviteJoinForm.invite_token.trim() || !inviteJoinForm.actor_id.trim()) {
      setError('Invite token and human id are required')
      return
    }
    try {
      const joined = await api.joinGroupByInvite({
        invite_token: inviteJoinForm.invite_token.trim(),
        actor_type: 'human',
        actor_id: inviteJoinForm.actor_id.trim(),
        capabilities: { ui: 'admin-human-session' },
      })
      safeStorage.setItem(`group_member_token_${id}`, joined.access_token)
      setMemberToken(joined.access_token)
      setEventForm(f => ({ ...f, sender_type: 'human', sender_id: joined.member.actor_id }))
      setArtifactForm(f => ({ ...f, created_by: joined.member.actor_id }))
      setInviteJoinForm(f => ({ ...f, invite_token: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Join by invite failed')
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
      if (invite.token) {
        setInviteJoinForm(f => ({ ...f, invite_token: invite.token || '' }))
      }
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
      const sendAsMember = memberToken && eventForm.sender_type === 'human'
      await api.streamGroupEvent(id, {
        event_type: 'message',
        sender_type: eventForm.sender_type,
        sender_id: eventForm.sender_id,
        content: eventForm.content,
      }, { onEvent: handleGroupStreamEvent }, sendAsMember ? memberToken : undefined)
      setEventForm(f => ({ ...f, content: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Send failed')
    } finally {
      setSendingEvent(false)
    }
  }

  const handleGroupStreamEvent = (streamEvent: GroupStreamEvent) => {
    switch (streamEvent.type) {
      case 'group.event':
        commitStreamEvent(streamEvent.event)
        break
      case 'group.agent_start':
        ensureStreamingEvent(streamEvent.sender_id, streamEvent.sender_type)
        break
      case 'group.agent_delta':
        appendStreamingDelta(streamEvent.sender_id, streamEvent.sender_type, streamEvent.text || '')
        break
      case 'group.artifact':
        commitStreamArtifact(streamEvent.artifact)
        break
      case 'group.done':
        setOrchestration(streamEvent.orchestration)
        ;(streamEvent.triggered || []).forEach(event => commitStreamEvent(event))
        break
      case 'group.error':
        setError(streamEvent.error || 'Group stream failed')
        break
    }
  }

  const ensureStreamingEvent = (senderID: string, senderType: string) => {
    if (!senderID) return
    const key = `${senderType}:${senderID}`
    setEvents(prev => {
      const existingID = streamingEventIdsRef.current[key]
      if (existingID && prev.some(event => event.id === existingID)) return prev
      const tempID = -Date.now() - Math.floor(Math.random() * 1000)
      streamingEventIdsRef.current[key] = tempID
      return [...prev, {
        id: tempID,
        group_id: id || '',
        event_type: 'message',
        sender_type: senderType,
        sender_id: senderID,
        content: '',
        metadata_json: '{"streaming":true}',
        created_at: new Date().toISOString(),
      }]
    })
  }

  const appendStreamingDelta = (senderID: string, senderType: string, text: string) => {
    if (!text) return
    const key = `${senderType}:${senderID}`
    setEvents(prev => {
      let tempID = streamingEventIdsRef.current[key]
      let next = prev
      if (!tempID || !prev.some(event => event.id === tempID)) {
        tempID = -Date.now() - Math.floor(Math.random() * 1000)
        streamingEventIdsRef.current[key] = tempID
        next = [...prev, {
          id: tempID,
          group_id: id || '',
          event_type: 'message',
          sender_type: senderType,
          sender_id: senderID,
          content: '',
          metadata_json: '{"streaming":true}',
          created_at: new Date().toISOString(),
        }]
      }
      return next.map(event => event.id === tempID ? { ...event, content: event.content + text } : event)
    })
  }

  const commitStreamEvent = (event: GroupEvent) => {
    const key = `${event.sender_type}:${event.sender_id}`
    setEvents(prev => {
      const tempID = streamingEventIdsRef.current[key]
      if (tempID) {
        delete streamingEventIdsRef.current[key]
      }
      const withoutTemp = tempID ? prev.filter(item => item.id !== tempID) : prev
      if (withoutTemp.some(item => item.id === event.id)) {
        return withoutTemp.map(item => item.id === event.id ? event : item)
      }
      return [...withoutTemp, event]
    })
  }

  const commitStreamArtifact = (artifact: GroupArtifact) => {
    if (!artifact?.id) return
    setArtifacts(prev => {
      if (prev.some(item => item.id === artifact.id)) {
        return prev.map(item => item.id === artifact.id ? artifact : item)
      }
      return [artifact, ...prev]
    })
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
      const createAsMember = memberToken && artifactForm.created_by === eventForm.sender_id
      await api.createGroupArtifact(id, {
        name: artifactForm.name,
        artifact_type: 'document',
        content: artifactForm.content,
        created_by: artifactForm.created_by,
      }, createAsMember ? memberToken : undefined)
      setArtifactForm(f => ({ ...f, content: '' }))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create artifact failed')
    }
  }

  const handleAddFlowStep = () => {
    const nextIndex = flowSteps.length + 1
    const nextStep = {
      id: `step-${nextIndex}`,
      name: `Step ${nextIndex}`,
      agent: memberAgentNames[0] || '',
      role: 'worker',
      system_prompt: '',
    }
    setFlowSteps(prev => [...prev, nextStep])
    setSelectedStep(flowSteps.length)
  }

  const updateFlowStep = (index: number, patch: Partial<FlowStep>) => {
    setFlowSteps(prev => prev.map((step, i) => i === index ? { ...step, ...patch } : step))
  }

  const moveFlowStep = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= flowSteps.length) return
    setFlowSteps(prev => {
      const next = [...prev]
      const current = next[index]
      next[index] = next[target]
      next[target] = current
      return next
    })
    setSelectedStep(target)
  }

  const deleteFlowStep = (index: number) => {
    setFlowSteps(prev => prev.filter((_, i) => i !== index))
    setSelectedStep(prev => Math.max(0, Math.min(prev, flowSteps.length - 2)))
  }

  const handleSaveFlow = async () => {
    if (!id || !group) return
    if (!token) {
      setError('Admin token required')
      return
    }
    setFlowSaving(true)
    setError('')
    try {
      const rules = parseObjectJson(group.rules_json)
      const workflow = rules.workflow && typeof rules.workflow === 'object' && !Array.isArray(rules.workflow)
        ? rules.workflow as Record<string, unknown>
        : {}
      const normalizedSteps = flowSteps.map((step, index) => ({
        id: step.id || `step-${index + 1}`,
        name: step.name || `Step ${index + 1}`,
        agent: step.agent,
        role: step.role || 'worker',
        system_prompt: step.system_prompt,
      }))
      const updatedRules = {
        ...rules,
        workflow: {
          ...workflow,
          type: 'manual',
          steps: normalizedSteps,
        },
      }
      await api.updateGroup(id, {
        name: group.name,
        description: group.description,
        orchestration_mode: group.orchestration_mode,
        rules: updatedRules,
        memory_policy: memoryPolicyForUpdate(group.memory_policy_json),
        status: group.status,
      }, token)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save flow failed')
    } finally {
      setFlowSaving(false)
    }
  }

  const handleStartTopic = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!group) return
    const content = topicForm.content.trim()
    if (!topicForm.entry_agent) {
      setError('Choose an entry agent before starting a topic')
      return
    }
    if (!content) {
      setError('Topic question is required')
      return
    }
    if (!token) {
      setError('Admin token required')
      return
    }
    setTopicStarting(true)
    setError('')
    try {
      const title = topicForm.title.trim() || content.slice(0, 50)
      const context = await api.createContext({ agent_name: topicForm.entry_agent, title })
      const params = new URLSearchParams({
        contextId: context.id,
        groupId: group.id,
        draft: content,
        autoSend: '1',
      })
      navigate(`/chat/${encodeURIComponent(topicForm.entry_agent)}?${params.toString()}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Start topic failed')
    } finally {
      setTopicStarting(false)
    }
  }

  if (loading) return <div className="p-8 text-sm text-[var(--text-tertiary)]">Loading...</div>
  if (!group) return <div className="p-8 text-sm text-[var(--error)]">Group not found</div>
  const groupTracePath = `/traces/context/${encodeURIComponent(`group:${group.id}`)}`
  const isP2PMode = group.orchestration_mode === 'p2p'
  const flowMode = isFlowMode(group.orchestration_mode)
  const selectedFlowStep = flowSteps[selectedStep]

  return (
    <div className="p-8 max-w-7xl">
      <div className="flex items-center justify-between mb-6">
        <button onClick={() => navigate('/groups')} className="flex items-center gap-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors">
          <ArrowLeft size={14} />
          Back to Groups
        </button>
        <div className="flex items-center gap-2">
          <Link to={groupTracePath} className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] no-underline">
            <ExternalLink size={14} />
            Trace
          </Link>
          <button onClick={load} className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            <RefreshCw size={14} />
            Refresh
          </button>
        </div>
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
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span className={`text-xs px-2 py-0.5 rounded-full ${actorColor(member.actor_type)}`}>{member.actor_type}</span>
                    <button
                      type="button"
                      onClick={() => handleRemoveMember(member)}
                      className="p-1 rounded text-[var(--text-tertiary)] hover:text-[var(--error)]"
                      title="Remove member"
                    >
                      <Trash2 size={13} />
                    </button>
                  </div>
                </div>
              ))}
            </div>

            <div className="mb-3">
              <label className="text-xs text-[var(--text-tertiary)]">Admin Token</label>
              <input
                type="password"
                value={token}
                onChange={e => { setToken(e.target.value); safeStorage.setItem('admin_token', e.target.value) }}
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
              <label className="text-xs text-[var(--text-tertiary)]">Admin add human</label>
              <input
                value={joinForm.client_id}
                onChange={e => setJoinForm({ client_id: e.target.value })}
                className="w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
              <button className="flex items-center justify-center gap-1.5 w-full px-3 py-1.5 text-sm rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
                <UserPlus size={14} />
                Add Human
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
                <div className="flex items-center justify-between mb-1">
                  <div className="text-xs text-[var(--text-tertiary)]">New invite token</div>
                  <button
                    type="button"
                    onClick={() => copyToClipboard(newInviteToken)}
                    className="p-1 rounded text-[var(--success)] hover:bg-[var(--success)]/20"
                    title="Copy token"
                  >
                    {copiedToken === newInviteToken ? <Check size={13} /> : <Copy size={13} />}
                  </button>
                </div>
                <code className="block text-xs break-all text-[var(--text-primary)] font-mono bg-[var(--bg-primary)]/50 rounded px-1.5 py-0.5">{newInviteToken}</code>
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

          <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4">
            <div className="flex items-center gap-2 mb-3">
              <KeyRound size={15} className="text-[var(--accent)]" />
              <h2 className="text-sm font-medium text-[var(--text-primary)]">Human Session</h2>
            </div>
            {memberToken && (
              <div className="mb-3 rounded-md border border-[var(--success)]/25 bg-[var(--success)]/10 p-2">
                <div className="text-xs text-[var(--success)] mb-1">Member token active for this browser</div>
                <div className="flex items-center gap-1.5">
                  <code className="flex-1 text-xs break-all text-[var(--text-primary)] font-mono bg-[var(--bg-primary)]/50 rounded px-1.5 py-0.5">{memberToken}</code>
                  <button
                    type="button"
                    onClick={() => copyToClipboard(memberToken)}
                    className="shrink-0 p-1 rounded text-[var(--success)] hover:bg-[var(--success)]/20"
                    title="Copy token"
                  >
                    {copiedToken === memberToken ? <Check size={13} /> : <Copy size={13} />}
                  </button>
                </div>
              </div>
            )}
            <form onSubmit={handleJoinByInvite} className="space-y-2">
              <input
                value={inviteJoinForm.actor_id}
                onChange={e => setInviteJoinForm(f => ({ ...f, actor_id: e.target.value }))}
                placeholder="human id"
                className="w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
              <input
                value={inviteJoinForm.invite_token}
                onChange={e => setInviteJoinForm(f => ({ ...f, invite_token: e.target.value }))}
                placeholder="invite token"
                className="w-full px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
              />
              <button className="flex items-center justify-center gap-1.5 w-full px-3 py-1.5 text-sm rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]">
                <KeyRound size={14} />
                Join With Invite
              </button>
            </form>
          </section>
        </aside>

        <div className="space-y-6 min-w-0">
          {(isP2PMode || flowMode) && (
            <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
              <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-[var(--border)] bg-[var(--bg-primary)]">
                <div className="min-w-0">
                  <h2 className="text-sm font-medium text-[var(--text-primary)]">Start Topic</h2>
                  <div className="mt-0.5 text-xs text-[var(--text-tertiary)] truncate">
                    Create a root context from this group and send the first question to an entry agent.
                  </div>
                </div>
                <span className="shrink-0 text-xs px-2 py-1 rounded-full bg-[var(--bg-tertiary)] text-[var(--text-secondary)]">root context</span>
              </div>
              <form onSubmit={handleStartTopic} className="space-y-3 p-4">
                <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
                  <div>
                    <label className="text-xs text-[var(--text-tertiary)]">Title</label>
                    <input
                      value={topicForm.title}
                      onChange={e => setTopicForm(form => ({ ...form, title: e.target.value }))}
                      placeholder="Optional topic title"
                      className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                    />
                  </div>
                  <div>
                    <label className="text-xs text-[var(--text-tertiary)]">Entry Agent</label>
                    <select
                      value={topicForm.entry_agent}
                      onChange={e => setTopicForm(form => ({ ...form, entry_agent: e.target.value }))}
                      className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                    >
                      <option value="">Choose agent</option>
                      {topicAgentOptions.map(name => <option key={name} value={name}>{name}</option>)}
                    </select>
                  </div>
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)]">Question</label>
                  <textarea
                    value={topicForm.content}
                    onChange={e => setTopicForm(form => ({ ...form, content: e.target.value }))}
                    rows={4}
                    placeholder={flowMode ? 'Describe the research question or workflow objective...' : 'Describe the direct P2P topic you want to start...'}
                    className="mt-1 w-full resize-none rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                  />
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <p className="text-xs text-[var(--text-tertiary)]">
                    The chat page will open with this group id attached, so the entry agent can discover group members and keep traces under the new root context.
                  </p>
                  <button
                    type="submit"
                    disabled={topicStarting || !topicForm.content.trim() || !topicForm.entry_agent}
                    className="flex shrink-0 items-center gap-1.5 rounded-md bg-[var(--accent)] px-3 py-2 text-sm text-white hover:bg-[var(--accent-hover)] disabled:opacity-50"
                  >
                    {topicStarting ? <RefreshCw size={14} className="animate-spin" /> : <Send size={14} />}
                    Start
                  </button>
                </div>
              </form>
            </section>
          )}

          {isP2PMode ? (
            <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
              <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-[var(--border)] bg-[var(--bg-primary)]">
                <div className="min-w-0">
                  <h2 className="text-sm font-medium text-[var(--text-primary)]">P2P Network</h2>
                  <div className="mt-0.5 text-xs text-[var(--text-tertiary)] truncate">{modeHint(group.orchestration_mode)}</div>
                </div>
                <span className="shrink-0 text-xs px-2 py-1 rounded-full bg-[var(--bg-tertiary)] text-[var(--text-secondary)]">{memberAgentNames.length} agents</span>
              </div>
              <div className="grid grid-cols-2 gap-4 p-5">
                {members.filter(member => member.actor_type === 'agent').map(member => (
                  <div key={member.actor_id} className="rounded-lg border border-[var(--border)] bg-[var(--bg-primary)] p-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--info)]/15 text-xs font-medium text-[var(--info)]">
                            {actorInitial(member.actor_id)}
                          </div>
                          <div className="min-w-0">
                            <div className="truncate text-sm font-medium text-[var(--text-primary)]">{member.actor_id}</div>
                            <div className="text-xs text-[var(--text-tertiary)]">{member.role}</div>
                          </div>
                        </div>
                      </div>
                      <Link
                        to={`/chat/${encodeURIComponent(member.actor_id)}?groupId=${encodeURIComponent(group.id)}`}
                        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]"
                        title="Open direct chat"
                      >
                        <MessageSquare size={15} />
                      </Link>
                    </div>
                    {member.capabilities_json && (
                      <pre className="mt-3 max-h-24 overflow-auto rounded bg-[var(--bg-tertiary)] p-2 text-xs text-[var(--text-secondary)]">
                        {tryFormatJson(member.capabilities_json)}
                      </pre>
                    )}
                  </div>
                ))}
                {memberAgentNames.length === 0 && (
                  <div className="col-span-2 rounded-lg border border-dashed border-[var(--border)] bg-[var(--bg-primary)] p-8 text-center text-sm text-[var(--text-tertiary)]">
                    No agents in this network
                  </div>
                )}
              </div>
            </section>
          ) : flowMode ? (
            <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
              <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-[var(--border)] bg-[var(--bg-primary)]">
                <div className="min-w-0">
                  <h2 className="text-sm font-medium text-[var(--text-primary)]">Manual Flow</h2>
                  <div className="mt-0.5 text-xs text-[var(--text-tertiary)] truncate">{modeHint(group.orchestration_mode)}</div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={handleAddFlowStep}
                    className="flex items-center gap-1.5 rounded-md bg-[var(--bg-tertiary)] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                  >
                    <ListOrdered size={14} />
                    Add Step
                  </button>
                  <button
                    type="button"
                    onClick={handleSaveFlow}
                    disabled={flowSaving}
                    className="flex items-center gap-1.5 rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white hover:bg-[var(--accent-hover)] disabled:opacity-50"
                  >
                    {flowSaving ? <RefreshCw size={14} className="animate-spin" /> : <Save size={14} />}
                    Save Flow
                  </button>
                </div>
              </div>
              <div className="grid grid-cols-[280px_1fr] min-h-[460px]">
                <div className="border-r border-[var(--border)] bg-[var(--bg-primary)] p-3">
                  <div className="space-y-2">
                    {flowSteps.map((step, index) => (
                      <button
                        key={`${step.id}-${index}`}
                        type="button"
                        onClick={() => setSelectedStep(index)}
                        className={`w-full rounded-md border px-3 py-2 text-left transition-colors ${
                          selectedStep === index
                            ? 'border-[var(--accent)] bg-[var(--accent)]/10'
                            : 'border-[var(--border)] bg-[var(--bg-secondary)] hover:bg-[var(--bg-tertiary)]/50'
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-xs text-[var(--text-secondary)]">{index + 1}</span>
                          <span className="min-w-0 truncate text-sm font-medium text-[var(--text-primary)]">{step.name}</span>
                        </div>
                        <div className="mt-1 truncate pl-8 text-xs text-[var(--text-tertiary)]">{step.agent || 'unassigned'} · {step.role}</div>
                      </button>
                    ))}
                    {flowSteps.length === 0 && (
                      <div className="rounded-md border border-dashed border-[var(--border)] p-6 text-center text-sm text-[var(--text-tertiary)]">No steps</div>
                    )}
                  </div>
                </div>
                <div className="p-5">
                  {selectedFlowStep ? (
                    <div className="space-y-4">
                      <div className="flex items-center justify-between gap-3">
                        <div>
                          <div className="text-xs text-[var(--text-tertiary)]">Step {selectedStep + 1}</div>
                          <h3 className="text-base font-medium text-[var(--text-primary)]">{selectedFlowStep.name}</h3>
                        </div>
                        <div className="flex items-center gap-1">
                          <button type="button" onClick={() => moveFlowStep(selectedStep, -1)} className="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]" title="Move up">
                            <ArrowUp size={14} />
                          </button>
                          <button type="button" onClick={() => moveFlowStep(selectedStep, 1)} className="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]" title="Move down">
                            <ArrowDown size={14} />
                          </button>
                          <button type="button" onClick={() => deleteFlowStep(selectedStep)} className="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-tertiary)] text-[var(--text-tertiary)] hover:text-[var(--error)]" title="Delete step">
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-xs text-[var(--text-tertiary)]">Name</label>
                          <input
                            value={selectedFlowStep.name}
                            onChange={e => updateFlowStep(selectedStep, { name: e.target.value })}
                            className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                          />
                        </div>
                        <div>
                          <label className="text-xs text-[var(--text-tertiary)]">Agent</label>
                          <select
                            value={selectedFlowStep.agent}
                            onChange={e => updateFlowStep(selectedStep, { agent: e.target.value })}
                            className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                          >
                            <option value="">Unassigned</option>
                            {selectedFlowStep.agent && !memberAgentNames.includes(selectedFlowStep.agent) && (
                              <option value={selectedFlowStep.agent}>{selectedFlowStep.agent} (not a member)</option>
                            )}
                            {memberAgentNames.map(name => <option key={name} value={name}>{name}</option>)}
                          </select>
                        </div>
                        <div>
                          <label className="text-xs text-[var(--text-tertiary)]">Role</label>
                          <input
                            value={selectedFlowStep.role}
                            onChange={e => updateFlowStep(selectedStep, { role: e.target.value })}
                            className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                          />
                        </div>
                        <div>
                          <label className="text-xs text-[var(--text-tertiary)]">ID</label>
                          <input
                            value={selectedFlowStep.id}
                            onChange={e => updateFlowStep(selectedStep, { id: e.target.value })}
                            className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)]"
                          />
                        </div>
                      </div>
                      <div>
                        <label className="text-xs text-[var(--text-tertiary)]">System Prompt</label>
                        <textarea
                          value={selectedFlowStep.system_prompt}
                          onChange={e => updateFlowStep(selectedStep, { system_prompt: e.target.value })}
                          rows={12}
                          className="mt-1 w-full resize-none rounded-md border border-[var(--border)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-sm text-[var(--text-primary)]"
                        />
                      </div>
                    </div>
                  ) : (
                    <div className="flex h-full items-center justify-center rounded-lg border border-dashed border-[var(--border)] text-sm text-[var(--text-tertiary)]">Add a step to start</div>
                  )}
                </div>
              </div>
            </section>
          ) : (
            <section className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg overflow-hidden">
              <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-[var(--border)] bg-[var(--bg-primary)]">
                <div className="min-w-0">
                  <h2 className="text-sm font-medium text-[var(--text-primary)]">Group Chat</h2>
                  <div className="mt-0.5 text-xs text-[var(--text-tertiary)] truncate">
                    {modeHint(group.orchestration_mode)}
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
                  {memberToken && eventForm.sender_type === 'human' && (
                    <span className="rounded-full bg-[var(--success)]/10 px-2 py-1 text-xs text-[var(--success)]">member token</span>
                  )}
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
          )}

          {!isP2PMode && (
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
          )}

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
