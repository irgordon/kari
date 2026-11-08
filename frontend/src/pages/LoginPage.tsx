import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { ApiError, apiGet, apiPost } from '../lib/api'
import type { AccountUser } from '../lib/api'

interface LoginResponse {
  message?: string
  user: AccountUser
}

export function LoginPage({ onSignedIn }: { onSignedIn: (user: LoginResponse['user']) => void }) {
  const location = useLocation()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [method, setMethod] = useState<'password' | 'magic' | 'reset'>('password')
  const [emailEnabled, setEmailEnabled] = useState(false)
  const [notice, setNotice] = useState('')
  useEffect(() => {
    void apiGet<{ enabled: boolean }>('/api/v1/auth/email/status').then(data => setEmailEnabled(data.enabled)).catch(() => setEmailEnabled(false))
  }, [])
  const sessionExpired = useMemo(
    () => new URLSearchParams(location.search).get('session') === 'expired',
    [location.search],
  )

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setIsLoading(true)
    setError(null)

    try {
      if (method !== 'password') {
        const result = await apiPost<{ email: string; purpose: string }, { message: string }>('/api/v1/auth/email/request', { email, purpose: method })
        setNotice(result.message)
        return
      }
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
        {method !== 'password' && <p>{method === 'reset' ? 'Request a password reset' : 'Request a sign-in link'}. Email access requires a verified account address.</p>}
        {notice && <p role="status">{notice}</p>}

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

          {method === 'password' && <label className="field">
            <span>Password</span>
            <input
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>}

          <button type="submit" disabled={isLoading}>
            {isLoading ? 'Please wait…' : method === 'password' ? 'Sign in' : 'Send email link'}
          </button>
        </form>
        <div className="inline-actions">
          {(['password', 'magic', 'reset'] as const).filter(value => value === 'password' || emailEnabled).map(value => <button key={value} type="button" disabled={isLoading || value === method} onClick={() => { setMethod(value); setError(null); setNotice(''); setPassword('') }}>{value === 'password' ? 'Use password' : value === 'magic' ? 'Use email link' : 'Forgot password?'}</button>)}
        </div>
      </div>
    </div>
  )
}
