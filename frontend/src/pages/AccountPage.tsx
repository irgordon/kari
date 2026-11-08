import { useEffect, useState } from 'react'
import { apiGet, apiPost } from '../lib/api'
import type { AccountUser } from '../lib/api'

export function AccountPage({ user }: { user: AccountUser }) {
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    void apiGet<{ enabled: boolean }>('/api/v1/auth/email/status').then(data => setEnabled(data.enabled)).catch(() => setError('Cannot check email availability. Try reloading the page.'))
  }, [])
  async function request(verify: boolean) {
    setBusy(true); setError(''); setMessage('')
    try {
      const response = verify
        ? await apiPost<object, { message: string }>('/api/v1/auth/email/verification/request', {})
        : await apiPost<object, { message: string }>('/api/v1/auth/email/request', { email: user.email, purpose: 'reset' })
      setMessage(response.message)
    } catch (error) { setError(error instanceof Error ? error.message : 'Cannot request email. Try again.') }
    finally { setBusy(false) }
  }
  return <section className="page stack">
    <h1>Your account</h1><p>{user.email}</p>
    <p>Email {user.email_verified ? 'verified' : 'not verified'}</p>
    {enabled === null && !error && <p role="status">Checking email availability…</p>}
    {enabled === false && <p>Email delivery is not configured. Contact your administrator to enable verification and account recovery.</p>}
    {message && <p role="status">{message}</p>}{error && <p role="alert">{error}</p>}
    {!user.email_verified && <button disabled={!enabled || busy} onClick={() => void request(true)}>Send verification email</button>}
    <button disabled={!enabled || !user.email_verified || busy} onClick={() => void request(false)}>Send password reset email</button>
  </section>
}
