import { FormEvent, useEffect, useMemo, useState } from 'react'
import { LogOut, Plus, Users } from 'lucide-react'
import Join from './pages/Join'
import Room from './pages/Room'
import { api, Group, Session } from './api/client'

const clientKey = 'a2a_human_client_id'

function groupsKey(clientId: string) {
  return `a2a_human_groups:${clientId}`
}

function activeGroupKey(clientId: string) {
  return `a2a_human_active_group:${clientId}`
}

interface LocalGroup {
  id: string
  name: string
  orchestration_mode: string
  status: string
  access_token: string
  joined_at: string
}

function loadGroups(clientId: string): LocalGroup[] {
  if (!clientId) return []
  const raw = localStorage.getItem(groupsKey(clientId))
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as LocalGroup[]
    return Array.isArray(parsed) ? parsed.filter(item => item.id && item.name && item.access_token) : []
  } catch {
    return []
  }
}

function saveGroups(clientId: string, groups: LocalGroup[]) {
  if (!clientId) return
  localStorage.setItem(groupsKey(clientId), JSON.stringify(groups))
}

function toLocalGroup(group: Group, accessToken: string): LocalGroup {
  return {
    id: group.id,
    name: group.name,
    orchestration_mode: group.orchestration_mode,
    status: group.status,
    access_token: accessToken,
    joined_at: new Date().toISOString(),
  }
}

export default function App() {
  const [clientId, setClientId] = useState(() => localStorage.getItem(clientKey) || '')
  const [groups, setGroups] = useState<LocalGroup[]>(() => loadGroups(localStorage.getItem(clientKey) || ''))
  const [activeGroupId, setActiveGroupId] = useState(() => {
    const savedClientId = localStorage.getItem(clientKey) || ''
    return savedClientId ? localStorage.getItem(activeGroupKey(savedClientId)) || '' : ''
  })
  const [joinOpen, setJoinOpen] = useState(false)
  const [groupToken, setGroupToken] = useState('')
  const [joining, setJoining] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = clientId ? 'A2A Human Client' : 'Join A2A'
  }, [clientId])

  useEffect(() => {
    saveGroups(clientId, groups)
  }, [clientId, groups])

  useEffect(() => {
    setGroups(loadGroups(clientId))
    setActiveGroupId(clientId ? localStorage.getItem(activeGroupKey(clientId)) || '' : '')
  }, [clientId])

  useEffect(() => {
    if (!clientId) return
    if (activeGroupId) {
      localStorage.setItem(activeGroupKey(clientId), activeGroupId)
    } else {
      localStorage.removeItem(activeGroupKey(clientId))
    }
  }, [activeGroupId, clientId])

  const activeGroup = useMemo(() => groups.find(group => group.id === activeGroupId) || null, [groups, activeGroupId])

  const joinGroup = async (event: FormEvent) => {
    event.preventDefault()
    const token = groupToken.trim()
    if (!token) {
      setError('Group token is required')
      return
    }
    setJoining(true)
    setError('')
    try {
      const join = await api.joinWithInvite(token, clientId)
      setGroups(prev => {
        const next = [toLocalGroup(join.group, join.access_token), ...prev.filter(item => item.id !== join.group.id)]
        return next
      })
      setActiveGroupId(join.group.id)
      setGroupToken('')
      setJoinOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to join group')
    } finally {
      setJoining(false)
    }
  }

  const leaveClient = () => {
    localStorage.removeItem(clientKey)
    setClientId('')
    setActiveGroupId('')
    setGroups([])
  }

  if (!clientId) {
    return (
      <Join
        onJoin={nextClientId => {
          localStorage.setItem(clientKey, nextClientId)
          setGroups(loadGroups(nextClientId))
          setActiveGroupId(localStorage.getItem(activeGroupKey(nextClientId)) || '')
          setClientId(nextClientId)
        }}
      />
    )
  }

  return (
    <main className="im-shell">
      <aside className="conversation-list">
        <div className="client-card">
          <div className="client-mark">{clientId.slice(0, 2).toUpperCase()}</div>
          <div className="client-meta">
            <strong>{clientId}</strong>
            <span>human client</span>
          </div>
          <button onClick={leaveClient} title="Switch client"><LogOut size={15} /></button>
        </div>

        <button className="add-group-button" onClick={() => setJoinOpen(value => !value)}>
          <Plus size={15} />
          Join Group
        </button>

        {joinOpen && (
          <form className="join-group-form" onSubmit={joinGroup}>
            <input
              value={groupToken}
              onChange={event => setGroupToken(event.target.value)}
              placeholder="invite token"
            />
            <button disabled={joining}>{joining ? 'Joining...' : 'Join'}</button>
          </form>
        )}

        {error && <div className="sidebar-error">{error}</div>}

        <div className="conversation-heading">Groups</div>
        <div className="conversation-items">
          {groups.length === 0 ? (
            <div className="empty-line">No groups joined</div>
          ) : groups.map(group => (
            <button
              key={group.id}
              className={`conversation-item ${group.id === activeGroupId ? 'active' : ''}`}
              onClick={() => setActiveGroupId(group.id)}
            >
              <div className="conversation-icon"><Users size={17} /></div>
              <div>
                <strong>{group.name}</strong>
                <span>{group.orchestration_mode}</span>
              </div>
            </button>
          ))}
        </div>
      </aside>

      {activeGroup ? (
        <Room session={{ client_id: clientId, group_id: activeGroup.id, access_token: activeGroup.access_token } satisfies Session} />
      ) : (
        <section className="empty-room">
          <h1>Select or join a group</h1>
          <p>Agents and humans are only discoverable after this client joins a group.</p>
        </section>
      )}
    </main>
  )
}
