import { Bot, UserRound } from 'lucide-react'
import { GroupMember } from '../api/client'

function memberIcon(type: string) {
  return type === 'agent' ? <Bot size={15} /> : <UserRound size={15} />
}

export default function MemberList({ members }: { members: GroupMember[] }) {
  const agents = members.filter(member => member.actor_type === 'agent')
  const humans = members.filter(member => member.actor_type === 'human')

  return (
    <div className="side-section">
      <div className="section-title">Participants</div>
      <div className="member-group">
        <div className="subhead">Agents</div>
        {agents.length === 0 ? <div className="empty-line">No agents</div> : agents.map(member => (
          <div className="member-row" key={`${member.actor_type}-${member.actor_id}`}>
            <div className="member-avatar agent">{memberIcon(member.actor_type)}</div>
            <div className="member-meta">
              <div>{member.actor_id}</div>
              <span>{member.role}</span>
            </div>
          </div>
        ))}
      </div>
      <div className="member-group">
        <div className="subhead">Humans</div>
        {humans.length === 0 ? <div className="empty-line">No humans</div> : humans.map(member => (
          <div className="member-row" key={`${member.actor_type}-${member.actor_id}`}>
            <div className="member-avatar human">{memberIcon(member.actor_type)}</div>
            <div className="member-meta">
              <div>{member.actor_id}</div>
              <span>{member.role}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
