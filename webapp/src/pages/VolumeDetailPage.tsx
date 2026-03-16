import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api'
import { formatBytes } from '../utils'
import type { VolumeDetail } from '../types'

export function VolumeDetailPage() {
  const { host = 'local', name = '' } = useParams()
  const [detail, setDetail] = useState<VolumeDetail>()
  const [error, setError] = useState<string>()

  useEffect(() => {
    api.volumeDetail(host, decodeURIComponent(name)).then(setDetail).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load detail'))
  }, [host, name])

  if (error) return <div className="alert error">{error}</div>
  if (!detail) return <div className="empty-card">Loading volume detail…</div>

  return (
    <div className="stack-lg">
      <section className="detail-hero panel-card">
        <div>
          <p className="eyebrow">Volume detail</p>
          <h2>{detail.summary.name}</h2>
          <p className="muted">{host} · {detail.summary.driver}</p>
        </div>
        <div className="hero-actions">
          <Link className="button" to={`/app/transfers/new?sourceHost=${encodeURIComponent(host)}&volume=${encodeURIComponent(detail.summary.name)}`}>Transfer volume</Link>
          <Link className="button ghost" to={`/app/volumes?host=${encodeURIComponent(host)}`}>Back to volumes</Link>
        </div>
      </section>
      <section className="stats-grid">
        <article className="stat-card"><span>Size</span><strong>{formatBytes(detail.size_bytes)}</strong></article>
        <article className="stat-card"><span>Attached</span><strong>{detail.summary.attached_containers_count}</strong></article>
        <article className="stat-card"><span>Running</span><strong>{detail.summary.running_containers}</strong></article>
      </section>
      <section className="panel-card">
        <div className="panel-heading"><h3>Attached containers</h3></div>
        <div className="container-grid">
          {detail.containers.length === 0 ? <div className="empty-card">No containers attached to this volume.</div> : detail.containers.map((container) => (
            <article className="container-card" key={container.id}>
              <strong className="container-name">{container.name}</strong>
              <span className="container-id">{container.id}</span>
              <span className={`container-status ${container.running ? 'health-good' : 'health-bad'}`}>{container.status}</span>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}
