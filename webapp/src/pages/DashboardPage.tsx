import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { JobList } from '../components/JobList'
import { formatDateTime } from '../utils'
import type { HostConfig, TransferJob } from '../types'

type HostHealth = { host: HostConfig; version?: string; error?: string }

export function DashboardPage() {
  const [jobs, setJobs] = useState<TransferJob[]>([])
  const [health, setHealth] = useState<HostHealth[]>([])
  const [error, setError] = useState<string>()

  useEffect(() => {
    Promise.all([api.hosts(), api.jobs('limit=8')])
      .then(async ([hostsResponse, jobsResponse]) => {
        setJobs(jobsResponse.jobs)
        const healthResults = await Promise.all(
          hostsResponse.hosts.map(async (host) => {
            try {
              const tested = await api.testHost(host.name)
              return { host, version: tested.version }
            } catch (err) {
              return { host, error: err instanceof Error ? err.message : 'Host check failed' }
            }
          }),
        )
        setHealth(healthResults)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load dashboard'))
  }, [])

  const running = jobs.filter((job) => ['queued', 'validating', 'running', 'cancelling'].includes(job.status))

  return (
    <div className="stack-lg">
      {error && <div className="alert error">{error}</div>}
      <section className="hero-card">
        <div>
          <p className="eyebrow">Operational overview</p>
          <h2>Move data without leaving the control plane</h2>
          <p className="muted">Track transfer jobs, inspect remote hosts, and launch copy or move operations with full history.</p>
        </div>
        <div className="hero-actions">
          <Link className="button" to="/app/transfers/new">Plan Transfer</Link>
          <Link className="button ghost" to="/app/volumes">Browse Volumes</Link>
        </div>
      </section>

      <section className="stats-grid">
        <article className="stat-card"><span>Hosts</span><strong>{health.length}</strong></article>
        <article className="stat-card"><span>Running jobs</span><strong>{running.length}</strong></article>
        <article className="stat-card"><span>Recent jobs</span><strong>{jobs.length}</strong></article>
      </section>

      <section className="panel-grid two-up">
        <article className="panel-card">
          <div className="panel-heading"><h3>Host health</h3><Link to="/app/hosts">Manage hosts</Link></div>
          <div className="host-health-list">
            {health.map((entry) => (
              <div className="host-health-card" key={entry.host.name}>
                <div>
                  <strong>{entry.host.name}</strong>
                  <p>{entry.host.kind === 'local' ? 'Local Docker host' : entry.host.host || entry.host.alias}</p>
                </div>
                <div className={entry.error ? 'health-bad' : 'health-good'}>
                  {entry.error ? entry.error : `Docker ${entry.version}`}
                </div>
              </div>
            ))}
          </div>
        </article>
        <article className="panel-card">
          <div className="panel-heading"><h3>Active transfers</h3><Link to="/app/transfers">Open history</Link></div>
          {running.length === 0 ? (
            <div className="empty-card">No active transfer jobs right now.</div>
          ) : (
            <div className="timeline-list">
              {running.map((job) => (
                <Link className="timeline-item" key={job.id} to={`/app/transfers/${job.id}`}>
                  <strong>{job.operation.toUpperCase()} {job.summary.total_items} item(s)</strong>
                  <span>{job.source_host} → {job.destination_host}</span>
                  <small>{formatDateTime(job.created_at)}</small>
                </Link>
              ))}
            </div>
          )}
        </article>
      </section>

      <section className="panel-card">
        <div className="panel-heading"><h3>Recent job history</h3><Link to="/app/transfers">View all</Link></div>
        <JobList jobs={jobs} />
      </section>
    </div>
  )
}
