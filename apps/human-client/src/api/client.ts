export interface Session {
  client_id: string
  group_id: string
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

const PLATFORM_BASE = trimSlash(import.meta.env.VITE_A2A_PLATFORM_URL?.trim() || '')
const directPlatform = PLATFORM_BASE !== ''

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${PLATFORM_BASE}${path}`, {
    headers: { 'content-type': 'application/json' },
    ...options,
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
  getGroup: (groupId: string) => request<Group>(`/api/groups/${encodeURIComponent(groupId)}`),
  joinGroup: (groupId: string, clientId: string) => request<GroupMember>(`/api/groups/${encodeURIComponent(groupId)}/join`, {
    method: 'POST',
    body: JSON.stringify({ client_id: clientId, capabilities: { ui: 'human-client' } }),
  }),
  listMembers: (groupId: string) => request<GroupMember[]>(`/api/groups/${encodeURIComponent(groupId)}/members`),
  listEvents: (groupId: string) => request<GroupEvent[]>(`/api/groups/${encodeURIComponent(groupId)}/events?limit=100`),
  listArtifacts: (groupId: string) => request<GroupArtifact[]>(`/api/groups/${encodeURIComponent(groupId)}/artifacts`),
  getOrchestration: (groupId: string) => request<GroupOrchestrationState>(`/api/groups/${encodeURIComponent(groupId)}/orchestration`),
  sendMessage: (groupId: string, clientId: string, content: string) => {
    const path = directPlatform
      ? `/api/groups/${encodeURIComponent(groupId)}/events`
      : `/api/groups/${encodeURIComponent(groupId)}/messages`
    return request<{ event: GroupEvent; orchestration: GroupOrchestrationState }>(path, {
      method: 'POST',
      body: JSON.stringify({
        event_type: 'message',
        sender_type: 'human',
        sender_id: clientId,
        content,
      }),
    })
  },
}

export async function loadRoom(groupId: string): Promise<RoomSnapshot> {
  const [group, members, events, artifacts, orchestration] = await Promise.all([
    api.getGroup(groupId),
    api.listMembers(groupId),
    api.listEvents(groupId),
    api.listArtifacts(groupId),
    api.getOrchestration(groupId),
  ])
  return { group, members, events, artifacts, orchestration }
}

function trimSlash(value: string) {
  return value.endsWith('/') ? value.slice(0, -1) : value
}
