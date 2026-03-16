import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import type { HostConfig, VolumeSummary } from '../types'

export function VolumesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const [hosts, setHosts] = useState<HostConfig[]>([])
  const [volumes, setVolumes] = useState<VolumeSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<string[]>(searchParams.getAll('volume'))
  const [search, setSearch] = useState('')
  const [error, setError] = useState<string>()
  const host = searchParams.get('host') || 'local'
  const hideAnonymous = searchParams.get('hideAnonymous') !== '0'

  useEffect(() => {
    api.hosts().then((data) => setHosts(data.hosts)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load hosts'))
  }, [])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(undefined)

    api.volumes(host, hideAnonymous)
      .then((data) => {
        if (cancelled) return
        setVolumes(data.volumes)
      })
      .catch((err) => {
        if (cancelled) return
        setVolumes([])
        setError(err instanceof Error ? err.message : 'Failed to load volumes')
      })
      .finally(() => {
        if (cancelled) return
        setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [host, hideAnonymous])

  const filtered = useMemo(() => volumes.filter((volume) => volume.name.toLowerCase().includes(search.toLowerCase())), [volumes, search])

  const toggleSelected = (name: string) => setSelected((current) => current.includes(name) ? current.filter((item) => item !== name) : [...current, name])

  return (
    <div className="stack-lg">
      {error && <div className="alert error">{error}</div>}
      <section className="panel-card">
        <div className="toolbar-grid">
          <label className="field">Host
            <select value={host} onChange={(e) => setSearchParams({ host: e.target.value, hideAnonymous: hideAnonymous ? '1' : '0' })}>
              {hosts.map((entry) => <option key={entry.name} value={entry.name}>{entry.name}</option>)}
            </select>
          </label>
          <label className="field">Search
            <input value={search} placeholder="Filter volume names" onChange={(e) => setSearch(e.target.value)} />
          </label>
          <label className="toggle-card"><input type="checkbox" checked={hideAnonymous} onChange={(e) => setSearchParams({ host, hideAnonymous: e.target.checked ? '1' : '0' })} /> Hide anonymous volumes</label>
        </div>
        {selected.length > 0 && (
          <div className="sticky-action-bar">
            <strong>{selected.length} selected</strong>
            <button className="button" onClick={() => {
              const params = new URLSearchParams({ sourceHost: host })
              selected.forEach((item) => params.append('volume', item))
              navigate(`/app/transfers/new?${params.toString()}`)
            }}>Open transfer builder</button>
          </div>
        )}
        <div className="table-wrap">
          <table className={`data-table${loading ? ' is-loading' : ''}`}>
            <thead><tr><th></th><th>Name</th><th>Driver</th><th>Attached</th><th>Running</th><th>Open</th></tr></thead>
            <tbody>
              {loading ? Array.from({ length: 6 }, (_, index) => (
                <tr key={`loading-${index}`} className="skeleton-row" aria-hidden="true">
                  <td><span className="skeleton-block skeleton-check" /></td>
                  <td><span className="skeleton-block skeleton-text long" /></td>
                  <td><span className="skeleton-block skeleton-text short" /></td>
                  <td><span className="skeleton-block skeleton-pill" /></td>
                  <td><span className="skeleton-block skeleton-pill" /></td>
                  <td><span className="skeleton-block skeleton-link" /></td>
                </tr>
              )) : filtered.map((volume) => (
                <tr key={volume.name}>
                  <td><input type="checkbox" checked={selected.includes(volume.name)} onChange={() => toggleSelected(volume.name)} /></td>
                  <td>{volume.name}</td>
                  <td>{volume.driver}</td>
                  <td>{volume.attached_containers_count}</td>
                  <td>{volume.running_containers}</td>
                  <td><Link to={`/app/volumes/${host}/${encodeURIComponent(volume.name)}`}>Inspect</Link></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
