import type { JobStatus } from '../types'

const labels: Record<JobStatus, string> = {
  queued: 'Queued',
  validating: 'Validating',
  running: 'Running',
  cancelling: 'Cancelling',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

export function StatusPill({ status }: { status: JobStatus }) {
  return <span className={`status-pill status-${status}`}>{labels[status]}</span>
}
