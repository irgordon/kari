import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiGet, apiPost } from '../lib/api'

type Site = { id: string; domain_name: string; runtime: string; repo_url: string; branch: string; status: string; ssl_status: string; expires_at: string; latest_deployment_id: string | null; latest_deployment_status: string | null }
type Deployment = { id: string; status: string; logs: string[] }

export function SitesPage() {
  const [sites, setSites] = useState<Site[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<Site | null>(null)
  const [deployment, setDeployment] = useState<{ site: string; value: Deployment } | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    apiGet<Site[]>('/api/v1/sites').then(data => { if (!cancelled) { setSites(data); setError('') } }).catch(e => { if (!cancelled) setError(e.message) })
    return () => { cancelled = true }
  }, [attempt])
  const siteID = deployment?.site
  const deploymentID = deployment?.value.id
  const status = deployment?.value.status
  useEffect(() => {
    if (!siteID || !deploymentID || !['pending', 'running'].includes(status ?? '')) return
    let cancelled = false
    const timer = window.setInterval(() => {
      apiGet<Deployment>(`/api/v1/sites/${siteID}/deployments/${deploymentID}`).then(value => {
        if (!cancelled) {
          setDeployment({ site: siteID, value })
          if (['success', 'failed'].includes(value.status)) setAttempt(v => v + 1)
        }
      }).catch(e => { if (!cancelled) setError(e.message) })
    }, 3000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [siteID, deploymentID, status])

  async function create(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    setBusy(true); setError('')
    try {
      await apiPost('/api/v1/sites', { domain_name: data.get('domain'), repo_url: data.get('repository'), branch: data.get('branch'), runtime: data.get('runtime') })
      form.reset(); setAttempt(v => v + 1)
    } catch (e) { setError(e instanceof Error ? e.message : 'Could not create site') }
    finally { setBusy(false) }
  }
  async function publish(site: Site) {
    setBusy(true); setError('')
    try {
      const value = await apiPost<object, { id: string }>(`/api/v1/sites/${site.id}/deploy`, {})
      setDeployment({ site: site.id, value: { ...value, status: 'pending', logs: [] } })
    } catch (e) { setError(e instanceof Error ? e.message : 'Could not queue publication') }
    finally { setBusy(false) }
  }
  async function tls(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    const form = event.currentTarget
    const data = new FormData(form)
    if (!data.get('certificate_pem') && !window.confirm('Request a certificate from Let’s Encrypt and agree to its subscriber agreement? DNS must point to this server and ports 80/443 must be reachable.')) return
    setBusy(true); setError('')
    try {
      await apiPost(`/api/v1/sites/${selected.id}/tls`, { certificate_pem: data.get('certificate_pem'), private_key_pem: data.get('private_key_pem'), agree_to_terms: !data.get('certificate_pem') })
      form.reset(); setSelected(null); setAttempt(v => v + 1)
    } catch (e) { setError(e instanceof Error ? e.message : 'Certificate activation failed') }
    finally { const key = form.elements.namedItem('private_key_pem') as HTMLTextAreaElement | null; if (key) key.value = ''; setBusy(false) }
  }
  return <div className="page stack">
    <header className="page-title"><h1>Websites</h1><p>Host static websites and Node.js applications from public HTTPS Git repositories.</p></header>
    <p>Static: prepared index.html and public assets, maximum 100 MiB/10,000 entries. Node.js: package-lock.json, optional build script, and npm start serving / on PORT=3000. Dependency install scripts are disabled. Configure environment and database before publishing.</p>
    {error && <div role="alert" className="alert error">{error}<button onClick={() => setAttempt(v => v + 1)}>Refresh sites</button></div>}
    <form className="card stack" onSubmit={create}>
      <h2>Create a website</h2>
      <label className="field">Domain<input name="domain" required placeholder="www.example.com" maxLength={253} /></label>
      <label className="field">Public Git repository<input name="repository" type="url" required placeholder="https://github.com/owner/prepared-site.git" maxLength={2048} /></label>
      <label className="field">Branch<input name="branch" required defaultValue="main" maxLength={100} /></label>
      <label className="field">Runtime<select name="runtime"><option value="static">Static website</option><option value="nodejs">Node.js 24.19.0</option></select></label>
      <button disabled={busy}>Create website</button>
    </form>
    <section className="card stack"><h2>Your websites</h2>
      {sites === null ? <p role="status">{error ? 'Website data is unavailable.' : 'Loading websites…'}</p> : sites.length === 0 ? <p>No websites yet. Create one above, then publish it.</p> : sites.map(site => <article className="card stack" key={site.id}>
        <h3>{site.domain_name}</h3><p>{site.runtime} · {site.repo_url} · {site.branch}</p>
        <p>Publication: {site.status} · Latest attempt: {site.latest_deployment_status ?? 'none'} · TLS: {site.ssl_status}{site.ssl_status === 'active' ? ` · expires ${new Date(site.expires_at).toLocaleDateString()}` : ''}</p>
        <p>Point this hostname’s A/AAAA records to this server. Do not leave an AAAA record pointing to a different server.</p>
        <div className="inline-actions">
          <Link to={`/sites/${site.id}`}>Manage website</Link>
          <button disabled={busy || (deployment?.site === site.id && ['pending', 'running'].includes(deployment.value.status))} onClick={() => void publish(site)}>Publish latest branch</button>
          <button disabled={busy || site.status !== 'running'} onClick={() => setSelected(site)}>Configure TLS</button>
          {site.latest_deployment_id && <button onClick={() => {
            apiGet<Deployment>(`/api/v1/sites/${site.id}/deployments/${site.latest_deployment_id}`).then(value => setDeployment({ site: site.id, value })).catch(e => setError(e.message))
          }}>View latest publication</button>}
          {site.status === 'running' && <a href={`${site.ssl_status === 'active' ? 'https' : 'http'}://${site.domain_name}`} target="_blank" rel="noreferrer">Visit website</a>}
        </div>
      </article>)}
    </section>
    {deployment && <section className="card stack" aria-live="polite"><h2>Publication: {deployment.value.status}</h2><p>Closing this page does not cancel publication.</p><pre>{deployment.value.logs.join('\n') || 'Waiting for the publisher…'}</pre></section>}
    {selected && <form className="card stack" onSubmit={tls}><h2>TLS for {selected.domain_name}</h2>
      <p>Leave both fields empty to request or renew through Let’s Encrypt (verified account email required). Alternatively, supply a certificate chain and matching PEM private key. The previous working certificate stays in place if activation fails.</p>
      <p><a href="https://letsencrypt.org/repository/" target="_blank" rel="noreferrer">Read the Let’s Encrypt subscriber agreement</a></p>
      <label className="field">Certificate chain (optional)<textarea name="certificate_pem" maxLength={65536} /></label>
      <label className="field">Private key (optional)<textarea name="private_key_pem" autoComplete="off" maxLength={16384} /></label>
      <button disabled={busy}>{busy ? 'Validating and activating…' : 'Activate TLS'}</button><button type="button" disabled={busy} onClick={() => setSelected(null)}>Cancel</button>
    </form>}
  </div>
}
