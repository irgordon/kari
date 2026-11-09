import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiGet } from '../lib/api'

type Site = { id: string; domain_name: string; status: string; ssl_status: string; expires_at: string }

export function DashboardPage() {
  const [sites, setSites] = useState<Site[] | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [observedAt] = useState(() => Date.now())
  useEffect(() => {
    let cancelled = false
    apiGet<Site[]>('/api/v1/sites').then(value => { if (!cancelled) { setSites(value); setError('') } }).catch(e => { if (!cancelled) setError(e.message) })
    return () => { cancelled = true }
  }, [attempt])
  const attention = sites?.filter(site => site.status !== 'running' || site.ssl_status !== 'active' || Date.parse(site.expires_at) < observedAt + 30 * 86400000)
  return <div className="page stack">
    <header className="page-title"><h1>Hosting overview</h1><p>Saved publication and certificate state for your latest 100 websites. These are not live uptime probes or host resource measurements.</p></header>
    {error && <div className="alert error" role="alert">Website data is unavailable: {error}<button onClick={() => setAttempt(v => v + 1)}>Retry</button></div>}
    {sites === null ? <p role="status">{error ? 'Unable to report hosting state.' : 'Loading hosting state…'}</p> : <>
      <section className="metrics-grid">
        <article className="card metric-card"><h2>Websites</h2><p className="metric-value">{sites.length}</p></article>
        <article className="card metric-card"><h2>Published</h2><p className="metric-value">{sites.filter(site => site.status === 'running').length}</p></article>
        <article className="card metric-card"><h2>Attention needed</h2><p className="metric-value">{attention?.length}</p></article>
      </section>
      <section className="card stack"><h2>Next steps</h2>
        {sites.length === 0 ? <p>Create and publish your first static website.</p> : attention?.length ? <ul>{attention.map(site => <li key={site.id}>{site.domain_name}: publication {site.status}, TLS {site.ssl_status}{site.ssl_status === 'active' ? ` (expires ${new Date(site.expires_at).toLocaleDateString()})` : ''}</li>)}</ul> : <p>No publication or certificate warnings in the saved records.</p>}
        <Link to="/sites">Manage websites</Link>
      </section>
    </>}
  </div>
}
