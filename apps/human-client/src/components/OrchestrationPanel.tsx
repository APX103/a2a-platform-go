import { GitBranch } from 'lucide-react'
import { Group, GroupOrchestrationState } from '../api/client'

function modeLabel(mode: string) {
  switch (mode) {
    case 'leader_led': return 'Leader-led'
    case 'roundtable': return 'Roundtable'
    case 'stateflow': return 'Stateflow'
    case 'research_long_horizon': return 'Research'
    default: return mode
  }
}

export default function OrchestrationPanel({
  group,
  orchestration,
}: {
  group: Group
  orchestration: GroupOrchestrationState | null
}) {
  return (
    <div className="side-section">
      <div className="section-title with-icon"><GitBranch size={15} /> Orchestration</div>
      <div className="mode-pill">{modeLabel(group.orchestration_mode)}</div>
      {orchestration && (
        <div className="orchestration-body">
          <div>
            <span>Next</span>
            <strong>{orchestration.next_action}</strong>
          </div>
          <div>
            <span>Speakers</span>
            <div className="speaker-list">
              {(orchestration.eligible_speakers || []).length === 0
                ? <em>none</em>
                : orchestration.eligible_speakers.map(speaker => <b key={speaker}>{speaker}</b>)}
            </div>
          </div>
          <div>
            <span>Context</span>
            <p>{orchestration.context_policy}</p>
          </div>
          <div>
            <span>Finish</span>
            <p>{orchestration.termination_policy}</p>
          </div>
        </div>
      )}
    </div>
  )
}
