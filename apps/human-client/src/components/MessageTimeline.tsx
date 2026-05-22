import { GroupEvent } from '../api/client'

function formatTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString()
}

export default function MessageTimeline({
  events,
  clientId,
}: {
  events: GroupEvent[]
  clientId: string
}) {
  if (events.length === 0) {
    return <div className="empty-timeline">No messages yet</div>
  }

  return (
    <div className="timeline">
      {events.map((event, index) => {
        const mine = event.sender_id === clientId
        return (
          <div className={`message ${mine ? 'mine' : ''}`} key={event.id || `${event.created_at}-${index}`}>
            <div className="message-head">
              <span>{event.sender_id}</span>
              <b>{event.sender_type}</b>
              <time>{formatTime(event.created_at)}</time>
            </div>
            <div className="message-body">{event.content}</div>
          </div>
        )
      })}
    </div>
  )
}
