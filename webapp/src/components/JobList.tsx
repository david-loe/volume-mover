import { Link } from 'react-router-dom'
import { formatBytes, formatDateTime } from '../utils'
import type { TransferJob } from '../types'
import { StatusPill } from './StatusPill'

export function JobList({ jobs }: { jobs: TransferJob[] }) {
  if (jobs.length === 0) {
    return <div className="empty-card">No jobs found.</div>
  }
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Job</th>
            <th>Path</th>
            <th>Status</th>
            <th>Items</th>
            <th>Data</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((job) => (
            <tr key={job.id}>
              <td><Link to={`/app/transfers/${job.id}`}>{job.operation.toUpperCase()}</Link></td>
              <td>{job.source_host} → {job.destination_host}</td>
              <td><StatusPill status={job.status} /></td>
              <td>{job.summary.completed_items}/{job.summary.total_items}</td>
              <td>{formatBytes(job.summary.bytes_copied || job.summary.bytes_estimated)}</td>
              <td>{formatDateTime(job.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
