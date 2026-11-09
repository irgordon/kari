import { useEffect, useState } from 'react'
import { apiGet, apiPost } from '../lib/api'

type Stats = { cpu_usage_percent: number; memory_total_bytes: number; memory_used_bytes: number; disk_total_bytes: number; disk_available_bytes: number; uptime_seconds: number; agent_version: string }
export function SystemPage() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [services, setServices] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    let cancelled = false
    Promise.all([apiGet<Stats>('/api/v1/system/stats'), apiGet<{ services: Record<string, string> }>('/api/v1/system/services')]).then(([metrics, state]) => {
      if (!cancelled) { setStats(metrics); setServices(state.services); setError('') }
    }).catch(e => { if (!cancelled) setError(e.message) })
    return () => { cancelled = true }
  }, [attempt])
  const gib = (bytes: number) => (bytes / 1073741824).toFixed(2)
  return <div className="page stack"><h1>Server operations</h1><p>Native-agent measurements taken when refreshed. Application logs and controls are under each website. Server management permission is required.</p>
    {error && <div role="alert" className="alert error">{error}</div>}
    <button onClick={() => setAttempt(v => v + 1)}>Refresh server state</button>
    {stats ? <section className="metrics-grid">
      <article className="card"><h2>CPU</h2><p>{stats.cpu_usage_percent.toFixed(1)}%</p></article>
      <article className="card"><h2>Memory</h2><p>{gib(stats.memory_used_bytes)} / {gib(stats.memory_total_bytes)} GiB</p></article>
      <article className="card"><h2>Root disk available</h2><p>{gib(stats.disk_available_bytes)} / {gib(stats.disk_total_bytes)} GiB</p></article>
      <article className="card"><h2>Uptime</h2><p>{Math.floor(stats.uptime_seconds / 3600)} hours · agent {stats.agent_version}</p></article>
    </section> : <p>Server measurements are unavailable.</p>}
    <section className="card stack"><h2>Host services</h2><ul>{Object.entries(services).map(([name, status]) => <li key={name}>{name}: {status}</li>)}</ul>
      <p>Only Nginx reload is exposed here; the configuration must validate first. SSH, Docker, and agent lifecycle remain local operator responsibilities.</p>
      <button disabled={busy} onClick={async () => { setBusy(true); try { await apiPost('/api/v1/system/services', { service: 'nginx', action: 'reload' }); setAttempt(v => v + 1) } catch (e) { setError(e instanceof Error ? e.message : 'Reload failed') } finally { setBusy(false) } }}>Validate and reload Nginx</button>
    </section>
  </div>
}
