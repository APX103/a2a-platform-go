export interface Session {
  human_id: string
  handle: string
  display_name: string
  session_token: string
  group_id: string
  access_token: string
}

export interface HumanSession {
  human_id: string
  handle: string
  display_name: string
  session_token: string
}

export interface HumanJoinPayload {
  session: HumanSession
  default_group?: Group
  default_access_token?: string
}

export interface Group {
  id: string
  name: string
  description?: string
  orchestration_mode: string
  rules_json?: string
  memory_policy_json?: string
  status: string
  created_at: string
  updated_at: string
}

export interface GroupMember {
  id: number
  group_id: string
  actor_type: 'agent' | 'human' | 'system' | string
  actor_id: string
  role: string
  capabilities_json?: string
  joined_at: string
  status?: string
}

export interface GroupEvent {
  id: number
  group_id: string
  event_type: string
  sender_type: 'agent' | 'human' | 'system' | string
  sender_id: string
  content: string
  metadata_json?: string
  created_at: string
}

export interface GroupArtifact {
  id: string
  group_id: string
  name: string
  artifact_type: string
  version: number
  content: string
  status: string
  created_by?: string
  created_at: string
  updated_at: string
}

export interface GroupOrchestrationState {
  group_id: string
  mode: string
  next_action: string
  eligible_speakers: string[]
  context_policy: string
  termination_policy: string
}

export interface RoomSnapshot {
  group: Group
  members: GroupMember[]
  events: GroupEvent[]
  artifacts: GroupArtifact[]
  orchestration: GroupOrchestrationState
}

export interface DirectAgentMessage {
  id: string
  sender: 'human' | 'agent'
  agent: string
  content: string
  reasoning_content?: string
  created_at: string
}

export interface GroupJoinResponse {
  group: Group
  member: GroupMember
  human?: {
    id: string
    handle: string
    display_name: string
  }
  access_token: string
  orchestration: GroupOrchestrationState
}

export interface HumanAuthResponse {
  human: {
    id: string
    handle: string
    display_name: string
  }
  session_token: string
  expires_at?: string
  created?: boolean
  default_group?: Group
  default_member?: GroupMember
  default_access_token?: string
  orchestration?: GroupOrchestrationState
}

const PLATFORM_BASE = trimSlash(import.meta.env.VITE_A2A_PLATFORM_URL?.trim() || '')
const directPlatform = PLATFORM_BASE !== ''

async function request<T>(path: string, options?: RequestInit & { accessToken?: string }): Promise<T> {
  const headers = {
    'content-type': 'application/json',
    ...(options?.accessToken ? { 'X-Group-Member-Token': options.accessToken } : {}),
    ...(options?.headers || {}),
  }
  const { accessToken: _accessToken, ...fetchOptions } = options || {}
  const res = await fetch(`${PLATFORM_BASE}${path}`, {
    ...fetchOptions,
    headers,
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`${res.status}: ${text}`)
  }
  const text = await res.text()
  return text ? JSON.parse(text) as T : undefined as T
}

export const api = {
  saveSession: (clientId: string) => directPlatform
    ? Promise.resolve({ client_id: clientId })
    : request<{ client_id: string }>('/api/session', {
      method: 'POST',
      body: JSON.stringify({ client_id: clientId }),
    }),
  registerHuman: (req: { handle: string; display_name?: string }) => request<HumanAuthResponse>('/api/humans/register', {
    method: 'POST',
    body: JSON.stringify(req),
  }),
  loginHuman: (req: { token?: string; handle?: string; display_name?: string }) => request<HumanAuthResponse>('/api/humans/login', {
    method: 'POST',
    body: JSON.stringify(req),
  }),
  getHumanMe: (sessionToken: string) => request<{ human: HumanAuthResponse['human'] }>('/api/humans/me', {
    headers: { 'X-Human-Session-Token': sessionToken },
  }),
  getGroup: (groupId: string, accessToken: string) => request<Group>(`/api/groups/${encodeURIComponent(groupId)}`, { accessToken }),
  joinWithInvite: (inviteToken: string, sessionToken: string) => request<GroupJoinResponse>('/api/group-joins', {
    method: 'POST',
    headers: { 'X-Human-Session-Token': sessionToken },
    body: JSON.stringify({
      invite_token: inviteToken,
      actor_type: 'human',
      capabilities: { ui: 'human-client' },
    }),
  }),
  listMembers: (groupId: string, accessToken: string) => request<GroupMember[]>(`/api/groups/${encodeURIComponent(groupId)}/members`, { accessToken }),
  listEvents: (groupId: string, accessToken: string) => request<GroupEvent[]>(`/api/groups/${encodeURIComponent(groupId)}/events?limit=100`, { accessToken }),
  listArtifacts: (groupId: string, accessToken: string) => request<GroupArtifact[]>(`/api/groups/${encodeURIComponent(groupId)}/artifacts`, { accessToken }),
  getOrchestration: (groupId: string, accessToken: string) => request<GroupOrchestrationState>(`/api/groups/${encodeURIComponent(groupId)}/orchestration`, { accessToken }),
  sendDirectToAgent: async (agentName: string, accessToken: string, humanId: string, content: string) => {
    const body = {
      jsonrpc: '2.0',
      id: `human-${Date.now()}`,
      method: 'SendStreamingMessage',
      params: {
        contextId: `human:${humanId}:agent:${agentName}`,
        message: {
          role: 'ROLE_USER',
          parts: [{ text: content }],
        },
      },
    }
    const res = await fetch(`${PLATFORM_BASE}/agent/${encodeURIComponent(agentName)}`, {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        accept: 'text/event-stream, application/json',
        'X-Group-Member-Token': accessToken,
        'X-A2A-Source-Agent': `human:${humanId}`,
      },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`${res.status}: ${text}`)
    }
    return readAgentResponse(res)
  },
  sendDirectToAgentStreaming: async (
    agentName: string,
    accessToken: string,
    humanId: string,
    content: string,
    callbacks: {
      onTextDelta?: (text: string) => void
      onThinkingDelta?: (text: string) => void
      onDone?: () => void
      onError?: (err: string) => void
    },
  ) => {
    const body = {
      jsonrpc: '2.0',
      id: `human-${Date.now()}`,
      method: 'SendStreamingMessage',
      params: {
        contextId: `human:${humanId}:agent:${agentName}`,
        message: {
          role: 'ROLE_USER',
          parts: [{ text: content }],
        },
      },
    }
    const res = await fetch(`${PLATFORM_BASE}/agent/${encodeURIComponent(agentName)}`, {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        accept: 'text/event-stream, application/json',
        'X-Group-Member-Token': accessToken,
        'X-A2A-Source-Agent': `human:${humanId}`,
      },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`${res.status}: ${text}`)
    }
    return readAgentResponseStreaming(res, callbacks)
  },
  sendMessage: (groupId: string, accessToken: string, clientId: string, content: string) => {
    const path = `/api/groups/${encodeURIComponent(groupId)}/events`
    return request<{ event: GroupEvent; orchestration: GroupOrchestrationState }>(path, {
      method: 'POST',
      accessToken,
      body: JSON.stringify({
        event_type: 'message',
        sender_type: 'human',
        sender_id: clientId,
        content,
      }),
    })
  },
}

export async function loadRoom(groupId: string, accessToken: string): Promise<RoomSnapshot> {
  const [group, members, events, artifacts, orchestration] = await Promise.all([
    api.getGroup(groupId, accessToken),
    api.listMembers(groupId, accessToken),
    api.listEvents(groupId, accessToken),
    api.listArtifacts(groupId, accessToken),
    api.getOrchestration(groupId, accessToken),
  ])
  return { group, members, events, artifacts, orchestration }
}

function trimSlash(value: string) {
  return value.endsWith('/') ? value.slice(0, -1) : value
}

async function readAgentResponse(res: Response): Promise<string> {
  const contentType = res.headers.get('content-type') || ''
  if (!contentType.includes('text/event-stream') || !res.body) {
    const text = await res.text()
    return extractJSONText(text) || text
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let finalText = ''
  while (true) {
    const { value, done } = await reader.read()
    if (value) {
      buffer += decoder.decode(value, { stream: !done })
      let frameEnd = buffer.indexOf('\n\n')
      while (frameEnd >= 0) {
        const frame = buffer.slice(0, frameEnd)
        buffer = buffer.slice(frameEnd + 2)
        finalText += extractSSEText(frame)
        frameEnd = buffer.indexOf('\n\n')
      }
    }
    if (done) break
  }
  if (buffer.trim()) {
    finalText += extractSSEText(buffer)
  }
  return finalText.trim() || 'No response text'
}

function extractSSEText(frame: string) {
  const data = frame.split('\n')
    .map(line => line.trim())
    .filter(line => line.startsWith('data:'))
    .map(line => line.slice(5).trim())
    .join('\n')
  return extractJSONText(data)
}

function extractJSONText(text: string): string {
  if (!text) return ''
  try {
    const value = JSON.parse(text)
    if (typeof value?.text === 'string') return value.text
    if (typeof value?.delta === 'string') return value.delta
    if (value?.type === 'text.delta' && typeof value?.text === 'string') return value.text
    const messageParts = value?.result?.message?.parts || value?.message?.parts
    if (Array.isArray(messageParts)) {
      return messageParts.map((part: { text?: string }) => part.text || '').join('')
    }
  } catch {
    return ''
  }
  return ''
}

function extractJSONThinking(text: string): string {
  if (!text) return ''
  try {
    const value = JSON.parse(text)
    if (value?.type === 'thinking.delta' && typeof value?.thinking === 'string') return value.thinking
  } catch {
    return ''
  }
  return ''
}

async function readAgentResponseStreaming(
  res: Response,
  callbacks: {
    onTextDelta?: (text: string) => void
    onThinkingDelta?: (text: string) => void
    onDone?: () => void
    onError?: (err: string) => void
  },
): Promise<void> {
  const contentType = res.headers.get('content-type') || ''
  if (!contentType.includes('text/event-stream') || !res.body) {
    const text = await res.text()
    const extracted = extractJSONText(text) || text
    callbacks.onTextDelta?.(extracted)
    callbacks.onDone?.()
    return
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (value) {
        buffer += decoder.decode(value, { stream: !done })
        let frameEnd = buffer.indexOf('\n\n')
        while (frameEnd >= 0) {
          const frame = buffer.slice(0, frameEnd)
          buffer = buffer.slice(frameEnd + 2)
          const data = extractSSEData(frame)
          const text = extractJSONText(data)
          if (text) callbacks.onTextDelta?.(text)
          const thinking = extractJSONThinking(data)
          if (thinking) callbacks.onThinkingDelta?.(thinking)
          frameEnd = buffer.indexOf('\n\n')
        }
      }
      if (done) break
    }
    if (buffer.trim()) {
      const data = extractSSEData(buffer)
      const text = extractJSONText(data)
      if (text) callbacks.onTextDelta?.(text)
      const thinking = extractJSONThinking(data)
      if (thinking) callbacks.onThinkingDelta?.(thinking)
    }
  } catch (err: any) {
    callbacks.onError?.(err?.message || 'Stream error')
  } finally {
    callbacks.onDone?.()
  }
}

function extractSSEData(frame: string): string {
  return frame.split('\n')
    .map(line => line.trim())
    .filter(line => line.startsWith('data:'))
    .map(line => line.slice(5).trim())
    .join('\n')
}
