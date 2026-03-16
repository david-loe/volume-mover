import { FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import type { HostConfig } from '../types'

const emptyForm = { name: '', kind: 'ssh', alias: '', host: '', user: '', port: 22, identityFile: '' }

export function HostsPage() {
  const [hosts, setHosts] = useState<HostConfig[]>([])
  const [form, setForm] = useState(emptyForm)
  const [flash, setFlash] = useState<string>()
  const [error, setError] = useState<string>()
  const [testing, setTesting] = useState<Record<string, string>>({})

  const load = () => api.hosts().then((data) => setHosts(data.hosts)).catch((err) => setError(err instanceof Error ? err.message : 'Failed to load hosts'))
  useEffect(() => { load() }, [])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    try {
      await api.saveHost(form)
      setFlash(`Saved host ${form.name}`)
      setForm(emptyForm)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save host')
    }
  }

  return (
    <div className="panel-grid wide-right">
      <section className="panel-card">
        <div className="panel-heading"><h3>Configured hosts</h3><button className="button ghost" onClick={() => api.importSSH().then(() => load()).then(() => setFlash('Imported SSH config')).catch((err) => setError(err instanceof Error ? err.message : 'Import failed'))}>Import SSH config</button></div>
        {flash && <div className="alert success">{flash}</div>}
        {error && <div className="alert error">{error}</div>}
        <div className="table-wrap">
          <table className="data-table">
            <thead><tr><th>Name</th><th>Target</th><th>Kind</th><th>Actions</th></tr></thead>
            <tbody>
              {hosts.map((host) => (
                <tr key={host.name}>
                  <td>{host.name}</td>
                  <td>{host.host || host.alias || 'localhost'}</td>
                  <td>{host.kind}</td>
                  <td>
                    <div className="inline-actions">
                      <button className="button ghost small" onClick={async () => {
                        try {
                          const result = await api.testHost(host.name)
                          setTesting((current) => ({ ...current, [host.name]: result.version }))
                        } catch (err) {
                          setTesting((current) => ({ ...current, [host.name]: err instanceof Error ? err.message : 'Test failed' }))
                        }
                      }}>Test</button>
                      {host.name !== 'local' && <button className="button ghost small" onClick={async () => { try { await api.deleteHost(host.name); await load() } catch (err) { setError(err instanceof Error ? err.message : 'Delete failed') } }}>Delete</button>}
                    </div>
                    {testing[host.name] && <div className="tiny-note">{testing[host.name]}</div>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel-card form-card">
        <div className="panel-heading"><h3>Add or update host</h3></div>
        <form className="form-grid" onSubmit={onSubmit}>
          <label>Name<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></label>
          <label>Kind<select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value })}><option value="ssh">ssh</option><option value="local">local</option></select></label>
          <label>Alias<input value={form.alias} onChange={(e) => setForm({ ...form, alias: e.target.value })} /></label>
          <label>Host<input value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} /></label>
          <label>User<input value={form.user} onChange={(e) => setForm({ ...form, user: e.target.value })} /></label>
          <label>Port<input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} /></label>
          <label>Identity File<input value={form.identityFile} onChange={(e) => setForm({ ...form, identityFile: e.target.value })} /></label>
          <button className="button" type="submit">Save Host</button>
        </form>
      </section>
    </div>
  )
}
