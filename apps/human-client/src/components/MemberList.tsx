import { Bot, UserRound } from 'lucide-react'
import { GroupMember } from '../api/client'

function memberIcon(type: string) {
  return type === 'agent' ? <Bot size={15} /> : <UserRound size={15} />
}

function parseCapabilities(member: GroupMember): Record<string, unknown> {
  if (!member.capabilities_json) return {}
  try {
    const parsed = JSON.parse(member.capabilities_json)
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

function displayName(member: GroupMember) {
  const caps = parseCapabilities(member)
  if (member.actor_type === 'human') {
    const name = typeof caps.display_name === 'string' && caps.display_name.trim() ? caps.display_name : ''
    const handle = typeof caps.handle === 'string' && caps.handle.trim() ? caps.handle : ''
    if (name && handle) return `${name} (@${handle})`
    return name || (handle ? `@${handle}` : member.actor_id)
  }
  return member.actor_id
}

export default function MemberList({
  members,
  activeAgent,
  onSelectAgent,
}: {
  members: GroupMember[]
  activeAgent?: string
  onSelectAgent?: (agentName: string) => void
}) {
  const agents = members.filter(member => member.actor_type === 'agent')
  const humans = members.filter(member => member.actor_type === 'human')

  return (
    <div className="side-section">
      <div className="section-title">Participants</div>
      <div className="member-group">
        <div className="subhead">Agents</div>
        {agents.length === 0 ? <div className="empty-line">No agents</div> : agents.map(member => (
          <div className={`member-row ${activeAgent === member.actor_id ? 'active' : ''}`} key={`${member.actor_type}-${member.actor_id}`}>
            <div className="member-avatar agent">{memberIcon(member.actor_type)}</div>
            <div className="member-meta">
              <div>{displayName(member)}</div>
              <span>{member.role}</span>
            </div>
            {onSelectAgent && (
              <button type="button" className="member-action" onClick={() => onSelectAgent(member.actor_id)}>
                Chat
              </button>
            )}
          </div>
        ))}
      </div>
      <div className="member-group">
        <div className="subhead">Humans</div>
        {humans.length === 0 ? <div className="empty-line">No humans</div> : humans.map(member => (
          <div className="member-row" key={`${member.actor_type}-${member.actor_id}`}>
            <div className="member-avatar human">{memberIcon(member.actor_type)}</div>
            <div className="member-meta">
              <div>{displayName(member)}</div>
              <span>{member.role}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
