import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'
import { StatusPill } from '../components/StatusPill'
import { formatBytes, formatDateTime } from '../utils'
import type { TransferJob, TransferJobEvent } from '../types'

export function JobDetailPage() {
  const { jobId = '' } = useParams()
  const navigate = useNavigate()
  const [job, setJob] = useState<TransferJob>()
  const [error, setError] = useState<string>()

  const load = () => api.job(jobId).then(setJob).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load job'))

  useEffect(() => { load() }, [jobId])
  useEffect(() => {
    if (!jobId) return
    const close = api.jobEvents(jobId, (_event: TransferJobEvent) => { void load() })
    return close
  }, [jobId])

  if (error) return <div className="alert error">{error}</div>
  if (!job) return <div className="empty-card">Loading job…</div>

  const canCancel = ['queued', 'validating', 'running', 'cancelling'].includes(job.status)

  return (
    <div className="stack-lg">
      <section className="detail-hero panel-card">
        <div>
          <p className="eyebrow">Transfer job</p>
          <h2>{job.operation.toUpperCase()} {job.summary.total_items} item(s)</h2>
          <p className="muted">{job.source_host} → {job.destination_host}</p>
        </div>
        <div className="hero-actions">
          <StatusPill status={job.status} />
          {canCancel && <button className="button ghost" onClick={async () => { await api.cancelJob(job.id); await load() }}>Cancel job</button>}
          <button className="button" onClick={() => navigate(`/app/transfers/new?fromJob=${encodeURIComponent(job.id)}`)}>Reopen as draft</button>
        </div>
      </section>
      <section className="stats-grid">
        <article className="stat-card"><span>Queued</span><strong>{job.summary.queued_items}</strong></article>
        <article className="stat-card"><span>Running</span><strong>{job.summary.running_items}</strong></article>
        <article className="stat-card"><span>Completed</span><strong>{job.summary.completed_items}</strong></article>
        <article className="stat-card"><span>Data</span><strong>{formatBytes(job.summary.bytes_copied || job.summary.bytes_estimated)}</strong></article>
      </section>
      <section className="panel-grid two-up">
        <article className="panel-card">
          <div className="panel-heading"><h3>Items</h3><span>{formatDateTime(job.created_at)}</span></div>
          <div className="timeline-list">
            {job.items.map((item) => (
              <div className="timeline-item static" key={`${item.index}-${item.source_volume}`}>
                <strong>{item.source_volume} → {item.destination_volume}</strong>
                <span>{item.status}</span>
                <small>{formatBytes(item.bytes_copied || item.bytes_estimated)}</small>
                {item.error && <small className="health-bad">{item.error}</small>}
              </div>
            ))}
          </div>
        </article>
        <article className="panel-card">
          <div className="panel-heading"><h3>Timeline</h3><Link to="/app/transfers">Back to history</Link></div>
          <div className="timeline-list">
            {(job.events || []).map((event) => (
              <div className="timeline-item static" key={event.id}>
                <strong>{event.step}</strong>
                <span>{event.message}</span>
                <small>{formatDateTime(event.created_at)}</small>
              </div>
            ))}
          </div>
        </article>
      </section>
    </div>
  )
}
