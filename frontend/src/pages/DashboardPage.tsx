import { useEffect, useState } from 'react'
import { apiGet } from '../lib/api'

interface SystemAlert {
  id: string
  severity: 'info' | 'warning' | 'critical'
  category: string
  message: string
  created_at: string
}

interface SystemStats {
  active_jails: number
  cpu_usage: number
  ram_usage: number
  uptime_seconds: number
}

const defaultStats: SystemStats = {
  active_jails: 0,
  cpu_usage: 0,
  ram_usage: 0,
  uptime_seconds: 0,
}

export function DashboardPage() {
  const [stats, setStats] = useState<SystemStats>(defaultStats)
  const [alerts, setAlerts] = useState<SystemAlert[]>([])

  useEffect(() => {
    let cancelled = false

    void Promise.all([
      apiGet<SystemAlert[]>('/api/v1/audit/alerts?status=unresolved'),
      apiGet<SystemStats>('/api/v1/system/stats'),
    ])
      .then(([alertsData, statsData]) => {
        if (cancelled) {
          return
        }
        setAlerts(alertsData)
        setStats(statsData)
      })
      .catch((error) => {
        console.error('Failed to load dashboard data', error)
      })

    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="page stack">
      <section className="metrics-grid">
        <article className="card metric-card">
          <h2>Active Jails</h2>
          <p className="metric-value">{stats.active_jails}</p>
        </article>
        <article className="card metric-card">
          <h2>CPU Load</h2>
          <p className="metric-value">{stats.cpu_usage}%</p>
        </article>
        <article className="card metric-card">
          <h2>Memory</h2>
          <p className="metric-value">{stats.ram_usage}%</p>
        </article>
        <article className="card metric-card">
          <h2>System Uptime</h2>
          <p className="metric-value">{formatUptime(stats.uptime_seconds)}</p>
        </article>
      </section>

      <section className="card stack">
        <div className="section-header">
          <h2>Priority Alerts</h2>
        </div>
        {alerts.length === 0 ? (
          <p className="muted">All systems operational.</p>
        ) : (
          <ul className="list stack">
            {alerts.map((alert) => (
              <li key={alert.id} className={`alert-item ${alert.severity}`}>
                <div className="alert-meta">
                  <span>{alert.category}</span>
                  <span>{new Date(alert.created_at).toLocaleTimeString()}</span>
                </div>
                <p>{alert.message}</p>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / (24 * 3600))
  const hours = Math.floor((seconds % (24 * 3600)) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return days > 0 ? `${days}d ${hours}h` : `${hours}h ${minutes}m`
}
