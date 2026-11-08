import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { apiPost } from '../lib/api'
import type { AccountUser, ApiPath } from '../lib/api'

export function EmailLinkPage({ onSignedIn, onReset }: { onSignedIn: (user: AccountUser) => void; onReset: () => void }) {
  const { purpose } = useParams()
  const navigate = useNavigate()
  const [token] = useState(() => new URLSearchParams(window.location.hash.slice(1)).get('token') ?? '')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { window.history.replaceState(window.history.state, '', window.location.pathname) }, [])
  const valid = purpose === 'verify' || purpose === 'reset' || purpose === 'magic'
  async function confirm(event: FormEvent) {
    event.preventDefault()
    if (!valid) return
    if (purpose === 'reset' && password !== confirmation) { setError('Passwords do not match.'); return }
    setBusy(true); setError('')
    try {
      const result = await apiPost<object, { user?: AccountUser; message?: string }>(`/api/v1/auth/email/${purpose}` as ApiPath, { token, password })
      setPassword(''); setConfirmation('')
      if (purpose === 'reset') { onReset(); setMessage(result.message ?? 'Password updated. Sign in again.') }
      else if (result.user) { onSignedIn(result.user); navigate(purpose === 'verify' ? '/account' : '/', { replace: true }) }
    } catch (error) { setError(error instanceof Error ? error.message : 'Cannot complete this action. Try again.') }
    finally { setBusy(false) }
  }
  return <main className="auth-page"><section className="auth-card">
    <h1>{purpose === 'verify' ? 'Verify your email' : purpose === 'reset' ? 'Set a new password' : 'Confirm sign-in'}</h1>
    {!valid || !token ? <p role="alert">Open a new link from your account email. This page has no valid link token.</p> : message ? <p role="status">{message}</p> : <>
      <p>This action takes effect only when you confirm below. Reloading this page removes the link token; reopen the email link if needed.</p>
      {purpose === 'verify' && <p>Stay signed in to the account that requested verification. If needed, <Link to="/login" target="_blank" rel="noopener noreferrer">sign in in another tab</Link>, then return here.</p>}
      {error && <p role="alert">{error}</p>}
      <form className="stack" onSubmit={confirm}>
        {purpose === 'reset' && <>
          <label className="field">New password<input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={event => setPassword(event.target.value)} /></label>
          <p>Use at least 12 characters with uppercase, lowercase, a number, and a symbol.</p>
          <label className="field">Confirm new password<input type="password" autoComplete="new-password" required value={confirmation} onChange={event => setConfirmation(event.target.value)} /></label>
        </>}
        <button disabled={busy}>{busy ? 'Please wait…' : purpose === 'verify' ? 'Verify email' : purpose === 'reset' ? 'Update password' : 'Confirm sign-in'}</button>
      </form>
    </>}
    <Link to="/login">Return to sign-in</Link>
  </section></main>
}
