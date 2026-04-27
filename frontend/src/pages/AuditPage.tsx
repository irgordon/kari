import { useEffect, useState } from 'react'
import { apiGet, apiPost } from '../lib/api'

interface AuditAlert {
  id: string
  severity: 'info' | 'warning' | 'critical'
  category: string
  message: string
  created_at: string
  is_resolved: boolean
}

const severityLabel: Record<string, string> = {
  critical: '🔴 Critical',
  warning: '🟡 Warning',
  info: '🔵 Info',
}

export function AuditPage() {
  const [alerts, setAlerts] = useState<AuditAlert[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [resolvingIds, setResolvingIds] = useState<Set<string>>(new Set())
  const [showResolved, setShowResolved] = useState(false)
  const [reloadTick, setReloadTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    const status = showResolved ? 'resolved' : 'unresolved'
    void apiGet<AuditAlert[]>(`/api/v1/audit/alerts?status=${status}`)
      .then((data) => {
        if (!cancelled) {
          setAlerts(data)
          setError(null)
          setLoading(false)
        }
      })
      .catch((e: unknown) => {
        console.error('Failed to load audit alerts', e)
        if (!cancelled) {
          setError('Failed to load audit alerts.')
          setLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [showResolved, reloadTick])

  async function resolveAlert(id: string) {
    if (resolvingIds.has(id)) return
    setResolvingIds((prev) => new Set(prev).add(id))
    try {
      await apiPost<Record<string, never>, void>(`/api/v1/audit/alerts/${id}/resolve`, {})
      setAlerts((prev) => prev.filter((a) => a.id !== id))
    } catch (e: unknown) {
      console.error('Failed to resolve alert', e)
      alert('Failed to dismiss alert. Please try again.')
    } finally {
      setResolvingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  return (
    <div className="page stack">
      <div className="page-title">
        <h1>Audit Logs</h1>
        <p>System alerts and security events from the Karı infrastructure.</p>
      </div>

      {error ? <div className="alert error">{error}</div> : null}

      <div className="section-header">
        <h2>Alerts</h2>
        <div className="inline-actions">
          <label className="field inline">
            <input
              type="checkbox"
              checked={showResolved}
              onChange={(e) => setShowResolved(e.target.checked)}
            />
            <span>Show resolved</span>
          </label>
          <button type="button" onClick={() => setReloadTick((t) => t + 1)}>
            Refresh
          </button>
        </div>
      </div>

      {loading ? (
        <p className="muted">Loading…</p>
      ) : alerts.length === 0 ? (
        <div className="card">
          <p className="muted center">
            {showResolved ? 'No resolved alerts found.' : 'All systems operational — no unresolved alerts.'}
          </p>
        </div>
      ) : (
        <div className="stack">
          {alerts.map((alert) => (
            <div key={alert.id} className={`card alert-item ${alert.severity}`}>
              <div className="alert-meta">
                <span>{severityLabel[alert.severity] ?? alert.severity}</span>
                <span className="muted small">{alert.category.toUpperCase().replace('_', ' ')}</span>
                <span className="muted small">{new Date(alert.created_at).toLocaleString()}</span>
              </div>
              <p>{alert.message}</p>
              {!alert.is_resolved ? (
                <div className="form-actions">
                  <button
                    type="button"
                    disabled={resolvingIds.has(alert.id)}
                    onClick={() => void resolveAlert(alert.id)}
                  >
                    {resolvingIds.has(alert.id) ? 'Resolving…' : 'Mark as Resolved'}
                  </button>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
