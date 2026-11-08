import { useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { ApiError, apiPost } from '../lib/api'

interface LoginResponse {
  message?: string
  user: { email: string }
}

export function LoginPage({ onSignedIn }: { onSignedIn: (user: LoginResponse['user']) => void }) {
  const location = useLocation()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const sessionExpired = useMemo(
    () => new URLSearchParams(location.search).get('session') === 'expired',
    [location.search],
  )

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setIsLoading(true)
    setError(null)

    try {
      const result = await apiPost<{ email: string; password: string }, LoginResponse>(
        '/api/v1/auth/login',
        { email, password },
      )
      setPassword('')
      onSignedIn(result.user)
      navigate('/', { replace: true })
    } catch (submitError) {
      setError(submitError instanceof ApiError ? submitError.message : 'Cannot connect. Please try again.')
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-brand">K</div>
        <h1>Sign in to Karı</h1>
        <p className="auth-subtitle">Platform-Agnostic Orchestration Engine</p>

        {error ? <div role="alert" className="alert error">{error}</div> : null}
        {new URLSearchParams(location.search).get('setup') === 'complete' && <p role="status">Administrator created. Sign in to continue.</p>}
        {!error && sessionExpired ? (
          <div className="alert warning">
            Your session expired. Please log in again.
          </div>
        ) : null}

        <form className="stack" onSubmit={onSubmit}>
          <label className="field">
            <span>Email address</span>
            <input
              type="email"
              autoComplete="email"
              required
              placeholder="admin@example.com"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </label>

          <label className="field">
            <span>Password</span>
            <input
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>

          <button type="submit" disabled={isLoading}>
            {isLoading ? 'Authenticating...' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  )
}
