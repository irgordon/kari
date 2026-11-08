import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiPost } from '../lib/api'

export function SetupPage({ onComplete }: { onComplete: () => void }) {
  const [token, setToken] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const navigate = useNavigate()
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (password !== confirmation) { setError('Passwords do not match.'); return }
    setBusy(true); setError('')
    try {
      await apiPost('/api/v1/setup/finalize', { admin_email: email, admin_password: password }, { headers: { 'X-Setup-Token': token, 'Content-Type': 'application/json' } })
      setToken(''); setPassword(''); setConfirmation('')
      onComplete(); navigate('/login?setup=complete', { replace: true })
    } catch (error) { setError(error instanceof Error ? error.message : 'Setup failed. Please try again.') }
    finally { setBusy(false) }
  }
  return <main className="auth-page"><section className="auth-card">
    <h1>Create administrator</h1>
    <p>Use the setup token from the API startup logs. It expires after 15 minutes; restart the API for a new token.</p>
    {error && <p className="alert error" role="alert">{error}</p>}
    <form className="stack" onSubmit={submit}>
      <label className="field">Setup token<input type="password" autoComplete="off" required value={token} onChange={event => setToken(event.target.value)} /></label>
      <label className="field">Email address<input type="email" autoComplete="email" required value={email} onChange={event => setEmail(event.target.value)} /></label>
      <label className="field">Password<input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={event => setPassword(event.target.value)} /></label>
      <p>Use 12–72 characters, including uppercase, lowercase, a number, and a symbol.</p>
      <label className="field">Confirm password<input type="password" autoComplete="new-password" required value={confirmation} onChange={event => setConfirmation(event.target.value)} /></label>
      <button disabled={busy} type="submit">{busy ? 'Creating administrator…' : 'Create administrator'}</button>
    </form>
  </section></main>
}
