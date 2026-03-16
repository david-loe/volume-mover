import { useEffect, useState } from 'react'
import { api } from '../api'
import { JobList } from '../components/JobList'
import type { HostConfig, TransferJob } from '../types'

export function JobsPage() {
  const [hosts, setHosts] = useState<HostConfig[]>([])
  const [jobs, setJobs] = useState<TransferJob[]>([])
  const [host, setHost] = useState('')
  const [status, setStatus] = useState('')
  const [operation, setOperation] = useState('')
  const [error, setError] = useState<string>()

  useEffect(() => { api.hosts().then((data) => setHosts(data.hosts)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load hosts')) }, [])
  useEffect(() => {
    const params = new URLSearchParams()
    if (host) params.set('host', host)
    if (status) params.set('status', status)
    if (operation) params.set('operation', operation)
    api.jobs(params.toString()).then((data) => setJobs(data.jobs)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load jobs'))
  }, [host, status, operation])

  return (
    <div className="stack-lg">
      {error && <div className="alert error">{error}</div>}
      <section className="panel-card">
        <div className="toolbar-grid three">
          <label className="field">Host<select value={host} onChange={(e) => setHost(e.target.value)}><option value="">All</option>{hosts.map((entry) => <option key={entry.name} value={entry.name}>{entry.name}</option>)}</select></label>
          <label className="field">Operation<select value={operation} onChange={(e) => setOperation(e.target.value)}><option value="">All</option><option value="copy">copy</option><option value="clone">clone</option><option value="move">move</option></select></label>
          <label className="field">Status<select value={status} onChange={(e) => setStatus(e.target.value)}><option value="">All</option><option value="queued">queued</option><option value="running">running</option><option value="completed">completed</option><option value="failed">failed</option><option value="cancelled">cancelled</option></select></label>
        </div>
      </section>
      <section className="panel-card">
        <div className="panel-heading"><h3>Transfer history</h3></div>
        <JobList jobs={jobs} />
      </section>
    </div>
  )
}
