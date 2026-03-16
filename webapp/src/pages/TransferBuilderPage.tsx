import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import type { CreateTransferJobRequest, HostConfig, TransferJob, TransferOperation, VolumeSummary } from '../types'

type Row = { sourceVolume: string; destinationVolume: string }

export function TransferBuilderPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [hosts, setHosts] = useState<HostConfig[]>([])
  const [sourceVolumes, setSourceVolumes] = useState<VolumeSummary[]>([])
  const [sourceHost, setSourceHost] = useState(searchParams.get('sourceHost') || 'local')
  const [destinationHost, setDestinationHost] = useState(searchParams.get('sourceHost') || 'local')
  const [operation, setOperation] = useState<TransferOperation>('copy')
  const [allowLive, setAllowLive] = useState(false)
  const [quiesceSource, setQuiesceSource] = useState(false)
  const [rows, setRows] = useState<Row[]>(() => {
    const selected = searchParams.getAll('volume')
    if (selected.length === 0) return [{ sourceVolume: '', destinationVolume: '' }]
    return selected.map((volume) => ({ sourceVolume: volume, destinationVolume: volume }))
  })
  const [error, setError] = useState<string>()

  useEffect(() => {
    api.hosts().then((data) => setHosts(data.hosts)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load hosts'))
  }, [])

  useEffect(() => {
    api.volumes(sourceHost, true).then((data) => setSourceVolumes(data.volumes)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load source volumes'))
  }, [sourceHost])

  useEffect(() => {
    const fromJob = searchParams.get('fromJob')
    if (!fromJob) return
    api.job(fromJob).then((job: TransferJob) => {
      setSourceHost(job.source_host)
      setDestinationHost(job.destination_host)
      setOperation(job.operation)
      setAllowLive(job.allow_live)
      setQuiesceSource(job.quiesce_source)
      setRows(job.items.map((item) => ({ sourceVolume: item.source_volume, destinationVolume: item.destination_volume })))
    }).catch((err) => setError(err instanceof Error ? err.message : 'Failed to prefill job'))
  }, [searchParams])

  const warnings = useMemo(() => {
    const items: string[] = []
    const seen = new Set<string>()
    rows.forEach((row, index) => {
      if (!row.sourceVolume || !row.destinationVolume) {
        items.push(`Row ${index + 1} needs both source and destination volumes.`)
      }
      if (sourceHost === destinationHost && row.sourceVolume === row.destinationVolume && row.sourceVolume) {
        items.push(`Row ${index + 1} must use a different destination name on the same host.`)
      }
      const key = `${destinationHost}:${row.destinationVolume}`
      if (row.destinationVolume) {
        if (seen.has(key)) items.push(`Destination volume ${row.destinationVolume} is duplicated.`)
        seen.add(key)
      }
    })
    if (operation === 'move' && quiesceSource) items.push('Quiesce is only used for clone/copy. It will be ignored for move jobs.')
    return items
  }, [rows, sourceHost, destinationHost, operation, quiesceSource])

  const submit = async () => {
    const payload: CreateTransferJobRequest = {
      operation,
      sourceHost,
      destinationHost,
      allowLive,
      quiesceSource,
      items: rows.map((row) => ({ sourceVolume: row.sourceVolume, destinationVolume: row.destinationVolume })),
    }
    try {
      const response = await api.createJob(payload)
      navigate(`/app/transfers/${response.jobId}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create transfer job')
    }
  }

  const updateRow = (index: number, patch: Partial<Row>) => setRows((current) => current.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row))
  const removeRow = (index: number) => setRows((current) => current.length === 1 ? [{ sourceVolume: '', destinationVolume: '' }] : current.filter((_, rowIndex) => rowIndex !== index))

  return (
    <div className="panel-grid two-up">
      <section className="panel-card">
        <div className="panel-heading"><h3>Transfer builder</h3><Link to="/app/transfers">View history</Link></div>
        {error && <div className="alert error">{error}</div>}
        <div className="toolbar-grid three transfer-config-grid">
          <label className="field">Operation<select value={operation} onChange={(e) => setOperation(e.target.value as TransferOperation)}><option value="copy">copy</option><option value="clone">clone</option><option value="move">move</option></select></label>
          <label className="field">Source host<select value={sourceHost} onChange={(e) => setSourceHost(e.target.value)}>{hosts.map((host) => <option key={host.name} value={host.name}>{host.name}</option>)}</select></label>
          <label className="field">Destination host<select value={destinationHost} onChange={(e) => setDestinationHost(e.target.value)}>{hosts.map((host) => <option key={host.name} value={host.name}>{host.name}</option>)}</select></label>
        </div>
        <div className="transfer-rows-modern transfer-row-list">
          {rows.map((row, index) => (
            <div className="transfer-row-card" key={`${index}-${row.sourceVolume}-${row.destinationVolume}`}>
              <label className="field">Source volume<select value={row.sourceVolume} onChange={(e) => updateRow(index, { sourceVolume: e.target.value, destinationVolume: row.destinationVolume || e.target.value })}><option value="">Select volume</option>{sourceVolumes.map((volume) => <option key={volume.name} value={volume.name}>{volume.name}</option>)}</select></label>
              <label className="field">Destination volume<input value={row.destinationVolume} onChange={(e) => updateRow(index, { destinationVolume: e.target.value })} placeholder="destination name" /></label>
              <button className="button ghost small" onClick={() => removeRow(index)}>Remove</button>
            </div>
          ))}
        </div>
        <div className="inline-actions spread transfer-actions">
          <button className="button ghost" onClick={() => setRows((current) => [...current, { sourceVolume: '', destinationVolume: '' }])}>Add volume</button>
          <button className="button" onClick={submit}>Queue transfer job</button>
        </div>
        <div className="transfer-options">
          <div className="transfer-option-row">
            <label className="transfer-option"><input type="checkbox" checked={allowLive} onChange={(e) => setAllowLive(e.target.checked)} /> <span>Allow live volumes with warning</span></label>
            <button type="button" className="info-icon" aria-label="About allowing live volumes" title="Allows the transfer to continue even when the source volume is attached to running containers. Use this only if application writes during the copy are acceptable.">
              i
              <span className="tooltip-bubble" role="tooltip">Allows the transfer to continue even when the source volume is attached to running containers. Use this only if application writes during the copy are acceptable.</span>
            </button>
          </div>
          <div className="transfer-option-row">
            <label className="transfer-option"><input type="checkbox" checked={quiesceSource} onChange={(e) => setQuiesceSource(e.target.checked)} /> <span>Stop source containers for copy or clone</span></label>
            <button type="button" className="info-icon" aria-label="About stopping source containers" title="Stops running containers that directly mount the source volume before the copy and starts those same containers again afterward. It does not manage the wider Compose stack.">
              i
              <span className="tooltip-bubble" role="tooltip">Stops running containers that directly mount the source volume before the copy and starts those same containers again afterward. It does not manage the wider Compose stack.</span>
            </button>
          </div>
        </div>
      </section>
      <section className="panel-card">
        <div className="panel-heading"><h3>Preflight</h3></div>
        {warnings.length === 0 ? <div className="alert success">No obvious validation issues detected.</div> : (
          <div className="warning-list">{warnings.map((warning) => <div className="alert warn" key={warning}>{warning}</div>)}</div>
        )}
        <div className="hint-card">
          <h4>Notes</h4>
          <ul>
            <li>Move jobs remove the source only after destination verification succeeds.</li>
            <li>Cancellation is best-effort and stops at the next safe checkpoint.</li>
            <li>Use the history view to reopen completed jobs as a draft.</li>
          </ul>
        </div>
      </section>
    </div>
  )
}
