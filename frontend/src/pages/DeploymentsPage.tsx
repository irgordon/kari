import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiGet, apiPost } from '../lib/api'

type ViewMode = 'list' | 'create' | 'terminal'
type DeploymentStatus = 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED'

interface Deployment {
  id: string
  domain_name: string
  status: DeploymentStatus
  branch: string
  created_at: string
}

interface CreateDeploymentPayload {
  name: string
  repo_url: string
  branch: string
  build_command: string
  target_port: number
  ssh_key?: string
}

interface CreateDeploymentResponse {
  trace_id: string
}

const statusLabels: Record<DeploymentStatus, string> = {
  PENDING: 'Queued',
  RUNNING: 'In Progress',
  SUCCESS: 'Stable',
  FAILED: 'Alert',
}

export function DeploymentsPage() {
  const sshKeyRef = useRef<HTMLTextAreaElement | null>(null)
  const [view, setView] = useState<ViewMode>('list')
  const [activeTraceId, setActiveTraceId] = useState<string | null>(null)
  const [deployments, setDeployments] = useState<Deployment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [logs, setLogs] = useState('')

  const [formData, setFormData] = useState<CreateDeploymentPayload>({
    name: '',
    repo_url: '',
    branch: 'main',
    build_command: 'npm install && npm run build',
    target_port: 3000,
  })

  async function onCreateDeployment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    try {
      const sshKey = sshKeyRef.current?.value.trim() ?? ''
      if (sshKeyRef.current) {
        sshKeyRef.current.value = ''
      }
      const payload: CreateDeploymentPayload = { ...formData }
      if (sshKey) {
        payload.ssh_key = sshKey
      }
      const result = await apiPost<CreateDeploymentPayload, CreateDeploymentResponse>(
        '/api/v1/apps/deploy',
        payload,
      )
      setActiveTraceId(result.trace_id)
      setLogs('')
      setView('terminal')
    } catch (createError) {
      console.error('Failed to create deployment', createError)
      setError('Deployment failed to initialize.')
    }
  }

  useEffect(() => {
    if (view !== 'list') {
      return undefined
    }

    let cancelled = false

    void apiGet<Deployment[]>('/api/v1/deployments')
      .then((data) => {
        if (!cancelled) {
          setDeployments(data)
        }
      })
      .catch((fetchError) => {
        console.error('Failed to fetch deployments', fetchError)
        if (!cancelled) {
          setError('Failed to load deployments.')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [view])

  useEffect(() => {
    if (view !== 'terminal' || !activeTraceId) {
      return
    }

    const stream = new EventSource(`/api/v1/deployments/${activeTraceId}/logs/stream`)
    stream.onmessage = (event) => {
      setLogs((current) => `${current}${event.data}\n`)
    }
    stream.onerror = (streamError) => {
      console.error('Deployment log stream error', streamError)
      stream.close()
    }

    return () => {
      stream.close()
    }
  }, [activeTraceId, view])

  return (
    <div className="page stack">
      <header className="page-header">
        <div>
          <h1>System Deployments</h1>
          <p>Orchestration Engine</p>
        </div>
        <div className="inline-actions">
          <button
            type="button"
            onClick={() => {
              setLoading(true)
              setError(null)
              setLogs('')
              setView('list')
            }}
          >
            List
          </button>
          <button type="button" onClick={() => setView('create')}>
            New App
          </button>
        </div>
      </header>

      {error ? <div className="alert error">{error}</div> : null}

      {view === 'list' ? (
        <section className="card">
          <div className="section-header">
            <h2>Recent Deployments</h2>
            {loading ? <span className="muted">Loading…</span> : null}
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Environment / App</th>
                  <th>Status</th>
                  <th>Branch</th>
                  <th>Initiated</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {deployments.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="muted center">
                      The Karı Muscle is idle. No deployments recorded.
                    </td>
                  </tr>
                ) : (
                  deployments.map((deployment) => (
                    <tr key={deployment.id}>
                      <td>
                        <div className="stack tight">
                          <span>{deployment.domain_name}</span>
                          <code>{deployment.id.slice(0, 8)}</code>
                        </div>
                      </td>
                      <td>{statusLabels[deployment.status] ?? deployment.status}</td>
                      <td>{deployment.branch}</td>
                      <td>{new Date(deployment.created_at).toLocaleString()}</td>
                      <td className="right">
                        <button
                          type="button"
                          onClick={() => {
                            setActiveTraceId(deployment.id)
                            setLogs('')
                            setView('terminal')
                          }}
                        >
                          View Console
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      {view === 'create' ? (
        <form className="card stack" onSubmit={onCreateDeployment}>
          <h2>Create New Application</h2>
          <p>Provision a new jail and proxy on the Karı Muscle.</p>

          <div className="form-grid">
            <label className="field">
              <span>App Name</span>
              <input
                required
                value={formData.name}
                onChange={(event) =>
                  setFormData((current) => ({ ...current, name: event.target.value }))
                }
              />
            </label>
            <label className="field">
              <span>Target Port</span>
              <input
                type="number"
                min={1024}
                max={65535}
                required
                value={formData.target_port}
                onChange={(event) =>
                  setFormData((current) => ({
                    ...current,
                    target_port: Number(event.target.value),
                  }))
                }
              />
            </label>
            <label className="field">
              <span>Repository URL</span>
              <input
                required
                value={formData.repo_url}
                onChange={(event) =>
                  setFormData((current) => ({
                    ...current,
                    repo_url: event.target.value,
                  }))
                }
              />
            </label>
            <label className="field">
              <span>Branch</span>
              <input
                required
                value={formData.branch}
                onChange={(event) =>
                  setFormData((current) => ({ ...current, branch: event.target.value }))
                }
              />
            </label>
          </div>

          <label className="field">
            <span>Build Command</span>
            <input
              required
              value={formData.build_command}
              onChange={(event) =>
                setFormData((current) => ({
                  ...current,
                  build_command: event.target.value,
                }))
              }
            />
          </label>

          <label className="field">
            <span>Private Deployment Key (SSH)</span>
            <textarea
              ref={sshKeyRef}
              rows={4}
            />
          </label>

          <div className="form-actions">
            <button type="submit">Initialize Deployment</button>
          </div>
        </form>
      ) : null}

      {view === 'terminal' && activeTraceId ? (
        <section className="card stack">
          <div className="section-header">
            <h2>Live Build Console</h2>
            <button
              type="button"
              onClick={() => {
                setLoading(true)
                setError(null)
                setLogs('')
                setView('list')
              }}
            >
              Close Console &amp; Return
            </button>
          </div>
          <p className="muted">{activeTraceId}</p>
          <pre className="terminal-output">{logs || 'Waiting for telemetry stream...'}</pre>
        </section>
      ) : null}
    </div>
  )
}
