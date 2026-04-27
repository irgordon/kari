import { useEffect, useRef, useState } from 'react'
import { apiGet } from '../lib/api'

interface SystemStats {
  active_jails: number
  cpu_usage: number
  ram_usage: number
  uptime_seconds: number
}

interface LogEntry {
  id: string
  level: string
  msg: string
  service: string
  ts: string
}

const defaultStats: SystemStats = {
  active_jails: 0,
  cpu_usage: 0,
  ram_usage: 0,
  uptime_seconds: 0,
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / (24 * 3600))
  const hours = Math.floor((seconds % (24 * 3600)) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return days > 0 ? `${days}d ${hours}h` : `${hours}h ${minutes}m`
}

function levelClass(level: string): string {
  switch (level) {
    case 'ERROR': return 'log-error'
    case 'WARN': return 'log-warn'
    case 'DEBUG': return 'log-debug'
    default: return 'log-info'
  }
}

export function SystemPage() {
  const [stats, setStats] = useState<SystemStats>(defaultStats)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [filterQuery, setFilterQuery] = useState('')
  const [selectedLevel, setSelectedLevel] = useState('ALL')
  const [isPaused, setIsPaused] = useState(false)
  const [statsError, setStatsError] = useState<string | null>(null)
  const isPausedRef = useRef(isPaused)
  useEffect(() => {
    isPausedRef.current = isPaused
  }, [isPaused])

  useEffect(() => {
    void apiGet<SystemStats>('/api/v1/system/stats')
      .then(setStats)
      .catch((err) => {
        console.error('Failed to load system stats', err)
        setStatsError('Failed to load system stats.')
      })
  }, [])

  useEffect(() => {
    const es = new EventSource('/api/v1/system/logs/stream')
    es.onmessage = (event: MessageEvent) => {
      if (isPausedRef.current) return
      try {
        const entry: LogEntry = JSON.parse(event.data as string)
        setLogs((prev) => [entry, ...prev].slice(0, 500))
      } catch {
        // ignore malformed log entries
      }
    }
    es.onerror = (err) => {
      console.error('System log stream error', err)
      es.close()
    }
    return () => {
      es.close()
    }
  }, [])

  const filteredLogs = logs.filter((log) => {
    const matchesQuery = log.msg.toLowerCase().includes(filterQuery.toLowerCase())
    const matchesLevel = selectedLevel === 'ALL' || log.level === selectedLevel
    return matchesQuery && matchesLevel
  })

  return (
    <div className="page stack">
      <div className="page-title">
        <h1>System</h1>
        <p>Live infrastructure telemetry and backplane log stream.</p>
      </div>

      {statsError ? <div className="alert error">{statsError}</div> : null}

      {/* Stats */}
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
          <h2>Uptime</h2>
          <p className="metric-value">{formatUptime(stats.uptime_seconds)}</p>
        </article>
      </section>

      {/* Live log stream */}
      <section className="card stack">
        <div className="section-header">
          <h2>System Backplane Logs</h2>
          <div className="inline-actions">
            <span className="muted small">
              {isPaused ? 'Paused' : '● Live'} · {filteredLogs.length} buffered
            </span>
            <button type="button" onClick={() => setIsPaused((p) => !p)}>
              {isPaused ? 'Resume' : 'Pause'}
            </button>
            <button type="button" onClick={() => setLogs([])}>
              Clear
            </button>
          </div>
        </div>

        <div className="form-grid">
          <label className="field">
            <span>Search</span>
            <input
              type="text"
              placeholder="Filter log messages…"
              value={filterQuery}
              onChange={(e) => setFilterQuery(e.target.value)}
            />
          </label>
          <label className="field">
            <span>Level</span>
            <select
              value={selectedLevel}
              onChange={(e) => setSelectedLevel(e.target.value)}
            >
              <option value="ALL">All Levels</option>
              <option value="INFO">Info</option>
              <option value="WARN">Warning</option>
              <option value="ERROR">Error</option>
              <option value="DEBUG">Debug</option>
            </select>
          </label>
        </div>

        <div className="terminal-output" style={{ height: '400px', overflowY: 'auto' }}>
          {filteredLogs.length === 0 ? (
            <span className="muted">Listening for system events…</span>
          ) : (
            filteredLogs.map((log) => (
              <div key={log.id} className={`log-line ${levelClass(log.level)}`}>
                <span className="log-ts">[{(() => { const d = new Date(log.ts); return isNaN(d.getTime()) ? log.ts : d.toLocaleTimeString() })()}]</span>
                <span className={`log-level ${levelClass(log.level)}`}>{log.level}</span>
                <span className="log-service">{log.service}:</span>
                <span className="log-msg">{log.msg}</span>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  )
}
