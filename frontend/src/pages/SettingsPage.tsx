import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { apiGet, apiPost } from '../lib/api'

interface SystemProfile {
  max_memory_per_app_mb: number
  max_cpu_percent_per_app: number
  version?: number
}

export function SettingsPage() {
  const [profile, setProfile] = useState<SystemProfile | null>(null)
  const [maxMemory, setMaxMemory] = useState(512)
  const [maxCpu, setMaxCpu] = useState(50)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  useEffect(() => {
    void loadProfile()
  }, [])

  async function loadProfile() {
    try {
      const data = await apiGet<SystemProfile>('/api/v1/system/profile')
      setProfile(data)
      setMaxMemory(data.max_memory_per_app_mb)
      setMaxCpu(data.max_cpu_percent_per_app)
    } catch (loadError) {
      console.error('Failed to load system profile', loadError)
      setError('Failed to load system profile.')
    }
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!profile || profile.version === undefined) {
      setError('System profile is not loaded yet. Please refresh and try again.')
      return
    }
    setIsSubmitting(true)
    setError(null)
    setSuccess(false)

    try {
      const payload = {
        max_memory_per_app_mb: Number(maxMemory),
        max_cpu_percent_per_app: Number(maxCpu),
        version: Number(profile.version),
      }
      await apiPost<typeof payload, SystemProfile>('/api/v1/system/profile', payload, {
        method: 'PUT',
      })
      setSuccess(true)
      void loadProfile()
    } catch (submitError) {
      console.error('Failed to update system profile', submitError)
      setError('Failed to update configuration.')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="page stack">
      <div className="page-title">
        <h1>System Governance</h1>
        <p>Configure global resource limits and SLA policies.</p>
      </div>

      {error ? <div className="alert error">{error}</div> : null}
      {success ? (
        <div className="alert success">
          System profile updated successfully. The Rust Agent is synchronizing
          state.
        </div>
      ) : null}

      <form className="card stack" onSubmit={onSubmit}>
        <div className="form-grid">
          <label className="field">
            <span>Max Memory Per App (MB)</span>
            <input
              type="number"
              min={128}
              required
              value={maxMemory}
              onChange={(event) => setMaxMemory(Number(event.target.value))}
            />
          </label>

          <label className="field">
            <span>Max CPU Allocation (%)</span>
            <input
              type="number"
              min={10}
              max={100}
              required
              value={maxCpu}
              onChange={(event) => setMaxCpu(Number(event.target.value))}
            />
          </label>
        </div>

        <div className="form-actions">
          <button
            type="submit"
            disabled={isSubmitting || !profile || profile.version === undefined}
          >
            {isSubmitting ? 'Saving...' : 'Save Configuration'}
          </button>
        </div>
      </form>
    </div>
  )
}
