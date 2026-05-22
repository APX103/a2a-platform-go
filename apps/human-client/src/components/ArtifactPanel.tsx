import { FileText } from 'lucide-react'
import { GroupArtifact } from '../api/client'

export default function ArtifactPanel({ artifacts }: { artifacts: GroupArtifact[] }) {
  return (
    <div className="side-section">
      <div className="section-title with-icon"><FileText size={15} /> Artifacts</div>
      {artifacts.length === 0 ? (
        <div className="empty-line">No artifacts</div>
      ) : (
        <div className="artifact-list">
          {artifacts.map(artifact => (
            <details key={artifact.id} className="artifact-item">
              <summary>
                <span>{artifact.name}</span>
                <b>v{artifact.version}</b>
              </summary>
              <div className="artifact-meta">{artifact.status}{artifact.created_by ? ` by ${artifact.created_by}` : ''}</div>
              <pre>{artifact.content}</pre>
            </details>
          ))}
        </div>
      )}
    </div>
  )
}
