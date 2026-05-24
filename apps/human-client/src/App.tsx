import { FormEvent, useEffect, useMemo, useState } from 'react'
import { LogOut, Plus, Users } from 'lucide-react'
import Join from './pages/Join'
import Room from './pages/Room'
import { api, Group, HumanJoinPayload, HumanSession, Session } from './api/client'

const sessionKey = 'a2a_human_session'

function groupsKey(humanId: string) {
  return `a2a_human_groups:${humanId}`
}

function activeGroupKey(humanId: string) {
  return `a2a_human_active_group:${humanId}`
}

interface LocalGroup {
  id: string
  name: string
  orchestration_mode: string
  status: string
  access_token: string
  joined_at: string
}

function loadHumanSession(): HumanSession | null {
  const raw = localStorage.getItem(sessionKey)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as HumanSession
    return parsed.human_id && parsed.handle && parsed.session_token ? parsed : null
  } catch {
    return null
  }
}

function saveHumanSession(session: HumanSession) {
  localStorage.setItem(sessionKey, JSON.stringify(session))
}

function loadGroups(humanId: string): LocalGroup[] {
  if (!humanId) return []
  const raw = localStorage.getItem(groupsKey(humanId))
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as LocalGroup[]
    return Array.isArray(parsed) ? parsed.filter(item => item.id && item.name && item.access_token) : []
  } catch {
    return []
  }
}

function saveGroups(humanId: string, groups: LocalGroup[]) {
  if (!humanId) return
  localStorage.setItem(groupsKey(humanId), JSON.stringify(groups))
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

function mergeDefaultGroup(groups: LocalGroup[], payload: HumanJoinPayload): LocalGroup[] {
  if (!payload.default_group || !payload.default_access_token) {
    return groups
  }
  const defaultGroup = toLocalGroup(payload.default_group, payload.default_access_token)
  return [defaultGroup, ...groups.filter(group => group.id !== defaultGroup.id)]
}

export default function App() {
  const [humanSession, setHumanSession] = useState<HumanSession | null>(() => loadHumanSession())
  const [groups, setGroups] = useState<LocalGroup[]>(() => loadGroups(loadHumanSession()?.human_id || ''))
  const [activeGroupId, setActiveGroupId] = useState(() => {
    const savedSession = loadHumanSession()
    return savedSession ? localStorage.getItem(activeGroupKey(savedSession.human_id)) || '' : ''
  })
  const [joinOpen, setJoinOpen] = useState(false)
  const [groupToken, setGroupToken] = useState('')
  const [joining, setJoining] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = humanSession ? 'A2A Human Client' : 'Join A2A'
  }, [humanSession])

  useEffect(() => {
    if (!humanSession?.session_token) return
    const ping = () => {
      api.getHumanMe(humanSession.session_token).catch(() => {})
    }
    ping()
    const timer = window.setInterval(ping, 30000)
    return () => window.clearInterval(timer)
  }, [humanSession?.session_token])

  useEffect(() => {
    saveGroups(humanSession?.human_id || '', groups)
  }, [humanSession?.human_id, groups])

  useEffect(() => {
    setGroups(loadGroups(humanSession?.human_id || ''))
    setActiveGroupId(humanSession ? localStorage.getItem(activeGroupKey(humanSession.human_id)) || '' : '')
  }, [humanSession])

  useEffect(() => {
    if (!humanSession) return
    if (activeGroupId) {
      localStorage.setItem(activeGroupKey(humanSession.human_id), activeGroupId)
    } else {
      localStorage.removeItem(activeGroupKey(humanSession.human_id))
    }
  }, [activeGroupId, humanSession])

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
      if (!humanSession) {
        setError('Human session is required')
        return
      }
      const join = await api.joinWithInvite(token, humanSession.session_token)
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
    localStorage.removeItem(sessionKey)
    setHumanSession(null)
    setActiveGroupId('')
    setGroups([])
  }

  if (!humanSession) {
    return (
      <Join
        onJoin={payload => {
          const nextSession = payload.session
          const nextGroups = mergeDefaultGroup(loadGroups(nextSession.human_id), payload)
          saveHumanSession(nextSession)
          saveGroups(nextSession.human_id, nextGroups)
          setGroups(nextGroups)
          setActiveGroupId(payload.default_group?.id || localStorage.getItem(activeGroupKey(nextSession.human_id)) || '')
          setHumanSession(nextSession)
        }}
      />
    )
  }

  return (
    <main className="im-shell">
      <aside className="conversation-list">
        <div className="client-card">
          <div className="client-mark">{humanSession.display_name.slice(0, 2).toUpperCase()}</div>
          <div className="client-meta">
            <strong>{humanSession.display_name}</strong>
            <span>@{humanSession.handle}</span>
          </div>
          <button onClick={leaveClient} title="Sign out"><LogOut size={15} /></button>
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
        <Room session={{
          human_id: humanSession.human_id,
          handle: humanSession.handle,
          display_name: humanSession.display_name,
          session_token: humanSession.session_token,
          group_id: activeGroup.id,
          access_token: activeGroup.access_token,
        } satisfies Session} />
      ) : (
        <section className="empty-room">
          <h1>Select or join a group</h1>
          <p>Agents and humans are only discoverable after this client joins a group.</p>
        </section>
      )}
    </main>
  )
}
