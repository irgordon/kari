import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { apiGet, apiPost } from '../lib/api'

type Site = { id: string; domain_name: string; runtime: string }
type FileEntry = { name: string; directory?: boolean; size?: number }
type Backup = { id: string; created_at: string }
type Runtime = { running?: boolean; logs?: string }

export function WebsitePage() {
  const { id } = useParams()
  const [site, setSite] = useState<Site | null>(null)
  const [files, setFiles] = useState<FileEntry[]>([])
  const [database, setDatabase] = useState(false)
  const [sftp, setSftp] = useState<{ enabled?: boolean; username?: string }>({})
  const [backups, setBackups] = useState<Backup[]>([])
  const [runtime, setRuntime] = useState<Runtime>({})
  const [path, setPath] = useState('')
  const [filename, setFilename] = useState('')
  const [text, setText] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const base = `/api/v1/sites/${id}` as const
  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const current = await apiGet<Site>(base)
        const [listing, db, access, saved, process] = await Promise.all([
          apiGet<{ entries?: FileEntry[] }>(`${base}/files?path=${encodeURIComponent(path)}`),
          apiGet<{ exists?: boolean }>(`${base}/database`),
          apiGet<{ enabled?: boolean; username?: string }>(`${base}/sftp`),
          apiGet<{ backups?: Backup[] }>(`${base}/backups`),
          current.runtime === 'nodejs' ? apiGet<Runtime>(`${base}/runtime`) : Promise.resolve({}),
        ])
        if (!cancelled) { setSite(current); setFiles(listing.entries ?? []); setDatabase(Boolean(db.exists)); setSftp(access); setBackups(saved.backups ?? []); setRuntime(process); setError('') }
      } catch (e) { if (!cancelled) setError(e instanceof Error ? e.message : 'Website management is unavailable') }
    }
    void load()
    return () => { cancelled = true }
  }, [base, path, attempt])

  async function operate(resource: string, input: object) {
    setBusy(true); setError(''); setNotice('')
    try {
      const result = await apiPost(`${base}/${resource}`, input)
      setAttempt(value => value + 1); setNotice('Operation completed. Review the refreshed state below.')
      return result
    } catch (e) { setError(e instanceof Error ? e.message : 'Operation failed'); return null }
    finally { setBusy(false) }
  }
  function confirmation(description: string) {
    return window.prompt(`${description} This can destroy current data. Type ${site?.domain_name} to confirm.`) ?? ''
  }
  async function environment(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const values: Record<string, string> = {}
    const raw = String(new FormData(form).get('environment') ?? '')
    for (const line of raw.split('\n').filter(line => line.trim() !== '')) {
      const equals = line.indexOf('=')
      if (equals < 1 || Object.hasOwn(values, line.slice(0, equals))) { setError('Use one unique KEY=value per line.'); return }
      values[line.slice(0, equals)] = line.slice(equals + 1)
    }
    await operate('runtime', { action: 'environment', environment: values }); form.reset()
  }
  async function read(name: string) {
    setError('')
    try {
      const target = path ? `${path}/${name}` : name
      const result = await apiGet<{ content?: string }>(`${base}/files?action=read&path=${encodeURIComponent(target)}`)
      const bytes = Uint8Array.from(atob(result.content ?? ''), char => char.charCodeAt(0))
      setText(new TextDecoder('utf-8', { fatal: true }).decode(bytes)); setFilename(target)
    } catch (e) { setError(e instanceof Error ? e.message : 'Use SFTP for binary or larger files') }
  }
  return <div className="page stack">
    <Link to="/sites">Back to websites</Link><h1>{site?.domain_name ?? 'Website management'}</h1>
    {error && <div className="alert error" role="alert">{error}<button onClick={() => setAttempt(v => v + 1)}>Refresh</button></div>}
    {notice && <p role="status">{notice}</p>}
    {site?.runtime === 'nodejs' && <section className="card stack"><h2>Node.js application</h2>
      <p>Current container: {runtime.running ? 'running' : 'not running'}. Publish from Websites after changing environment or creating/deleting a database.</p>
      <div className="inline-actions">{['start', 'stop', 'restart'].map(action => <button key={action} disabled={busy} onClick={() => void operate('runtime', { action })}>{action}</button>)}
        <button disabled={busy} onClick={() => { apiGet<Runtime>(`${base}/runtime?action=logs`).then(setRuntime).catch(e => setError(e.message)) }}>View recent application logs</button>
      </div><pre>{runtime.logs}</pre>
      <form className="stack" onSubmit={environment}><h3>Replace application environment</h3><p>Values are write-only and saved in protected host files. Empty input clears custom variables. PORT, HOST, NODE_ENV, KARI_DATA_DIR, and DATABASE_URL are managed by Karı.</p>
        <label className="field">Environment (KEY=value, one per line)<textarea name="environment" autoComplete="off" maxLength={131072} /></label><button disabled={busy}>Save environment for next publication</button>
      </form>
    </section>}
    <section className="card stack"><h2>Customer PostgreSQL database</h2><p>{database ? 'Database container exists. Applications connect as non-superuser site to database:5432/site on this website’s private network. DATABASE_URL is injected at the next publication.' : 'No customer database container. The panel database is separate.'}</p>
      {database ? <button disabled={busy} onClick={() => { const confirm = confirmation('Delete this website’s customer database?'); if (confirm === site?.domain_name) void operate('database', { action: 'delete', confirm }) }}>Delete customer database</button> : <button disabled={busy || !site} onClick={() => void operate('database', { action: 'create' })}>Create customer database</button>}
    </section>
    <section className="card stack"><h2>Site data files</h2><p>Persistent data is /data in Node applications and public under /files/ for static websites. Browser editing supports UTF-8 text up to 256 KiB; use SFTP for larger/binary files. Published Git releases are read-only.</p>
      <label className="field">Directory (relative to site data)<input value={path} onChange={event => setPath(event.target.value)} /></label>
      <ul>{files.map(file => <li key={file.name}><button onClick={() => file.directory ? setPath(path ? `${path}/${file.name}` : file.name) : void read(file.name)}>{file.directory ? 'Directory: ' : 'File: '}{file.name}</button> {file.size ?? 0} bytes</li>)}</ul>
      <label className="field">File or new directory path<input value={filename} onChange={event => setFilename(event.target.value)} /></label>
      <div className="field"><label htmlFor="site-file-text">Text contents</label><textarea id="site-file-text" value={text} onChange={event => setText(event.target.value)} /></div>
      <div className="inline-actions"><button disabled={busy || !filename} onClick={() => {
        const bytes = new TextEncoder().encode(text)
        if (bytes.length > 262144) { setError('Browser file limit is 256 KiB.'); return }
        let binary = ''; for (const value of bytes) binary += String.fromCharCode(value)
        void operate('files', { action: 'write', path: filename, content: btoa(binary) })
      }}>Save file</button><button disabled={busy || !filename} onClick={() => void operate('files', { action: 'mkdir', path: filename })}>Create directory</button>
        <button disabled={busy || !filename} onClick={() => { const confirm = confirmation(`Delete file ${filename}?`); if (confirm === site?.domain_name) void operate('files', { action: 'delete', path: filename, confirm }) }}>Delete file</button></div>
    </section>
    <section className="card stack"><h2>SFTP access</h2><p>{sftp.enabled ? 'Key access enabled' : 'Key access disabled'} · {sftp.username}</p><p>Connect with <code>sftp {sftp.username}@{window.location.hostname}</code> on port 22. The session is confined to this website and starts in /files; shell and forwarding are disabled.</p>
      <form onSubmit={event => { event.preventDefault(); const form = event.currentTarget; void operate('sftp', { action: 'enable', public_key: new FormData(form).get('key') }).finally(() => form.reset()) }}>
        <label className="field">OpenSSH public key<input name="key" required maxLength={16384} placeholder="ssh-ed25519 AAAA…" /></label><button disabled={busy}>Enable or replace SFTP key</button>
      </form><button disabled={busy || !sftp.enabled} onClick={() => void operate('sftp', { action: 'disable' })}>Disable SFTP key access</button>
    </section>
    <section className="card stack"><h2>Manual workload backups</h2><p>Creation briefly pauses the application and SFTP sessions. A snapshot contains site files, customer database, environment, release, and certificate. Restore replaces current data and restarts the saved application. Failure during replacement leaves the site/SFTP stopped for operator repair; inspect state if completion is uncertain. Backups stay on this server; off-host automation remains separate.</p>
      <button disabled={busy} onClick={() => void operate('backups', { action: 'create' })}>Create workload backup</button>
      <ul>{backups.map(backup => <li key={backup.id}>{new Date(backup.created_at).toLocaleString()} · {backup.id}
        <button disabled={busy} onClick={() => { const confirm = confirmation('Restore this snapshot, replacing files/database/settings?'); if (confirm === site?.domain_name) void operate('backups', { action: 'restore', backup_id: backup.id, confirm }) }}>Restore</button>
        <button disabled={busy} onClick={() => { const confirm = confirmation('Permanently delete this snapshot?'); if (confirm === site?.domain_name) void operate('backups', { action: 'delete', backup_id: backup.id, confirm }) }}>Delete backup</button>
      </li>)}</ul>
    </section>
  </div>
}
