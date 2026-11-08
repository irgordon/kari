import { useEffect, useState } from 'react'
import { apiGet, apiPost } from '../lib/api'
import type { AccountUser } from '../lib/api'

export function AccountPage({ user, onUserChange }: { user: AccountUser; onUserChange: (user: AccountUser) => void }) {
  const [onboarding, setOnboarding] = useState<{ checks: Record<string, boolean>; complete: boolean; version: string; agent_required: boolean } | null>(null)
  const [mfaEnabled, setMfaEnabled] = useState(user.mfa_enabled)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [enrollment, setEnrollment] = useState<{ secret: string; qr: string } | null>(null)
  const [recovery, setRecovery] = useState<string[]>([])
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    void apiGet<{ enabled: boolean }>('/api/v1/auth/email/status').then(data => setEnabled(data.enabled)).catch(() => setError('Cannot check email availability. Try reloading the page.'))
  }, [])
  useEffect(() => {
    void apiGet<{ checks: Record<string, boolean>; complete: boolean; version: string; agent_required: boolean }>('/api/v1/auth/onboarding').then(setOnboarding).catch(() => setError('Cannot load onboarding checks. Reload to try again.'))
  }, [user])
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
  async function mfaAction(action: 'begin' | 'enable' | 'disable') {
    setBusy(true); setError('')
    try {
      const result = await apiPost<object, { secret?: string; qr?: string; recovery_codes?: string[]; user?: AccountUser }> (`/api/v1/auth/mfa/${action}`, { password, code })
      if (action === 'begin') setEnrollment({ secret: result.secret!, qr: result.qr! })
      else { if (result.user) onUserChange(result.user); setMfaEnabled(action === 'enable'); setEnrollment(null); setPassword(''); setCode(''); setRecovery(result.recovery_codes ?? []); setMessage(action === 'enable' ? 'MFA enabled. Save your recovery codes offline.' : 'MFA disabled. Production management remains blocked until you enroll again.') }
    } catch (error) { setError(error instanceof Error ? error.message : 'MFA action failed.') }
    finally { setBusy(false) }
  }
  return <section className="page stack">
    <h1>Your account</h1><p>{user.email}</p>
    <h2>Server onboarding</h2>
    {onboarding ? <><p>Release {onboarding.version}: {onboarding.complete ? 'Onboarding complete' : 'Setup actions remain'}</p><ul>{Object.entries(onboarding.checks).map(([name, ok]) => <li key={name}>{name.replaceAll('_', ' ')}: {ok ? 'Ready' : name === 'native_agent' && !onboarding.agent_required ? 'Not connected (optional in development)' : 'Action required'}</li>)}</ul><p>Configure HTTPS and the native agent with the installer, verify your email below, and enable your authenticator. Missing checks do not mean the server is healthy.</p></> : <p>Loading onboarding checks…</p>}
    <h2>Authenticator protection</h2>
    <p>MFA {mfaEnabled ? 'enabled' : 'not enabled'}. {user.mfa_required && !mfaEnabled && 'Enroll before managing production workloads.'}</p>
    <label className="field">Current password<input type="password" autoComplete="current-password" value={password} onChange={event => setPassword(event.target.value)} /></label>
    {enrollment && <><img src={enrollment.qr} alt="Authenticator enrollment QR code" /><p>Manual key: <code>{enrollment.secret}</code></p><p>Scan this key with your authenticator. Enrollment expires after ten minutes.</p></>}
    {(enrollment || mfaEnabled) && <label className="field">Authenticator code<input autoComplete="one-time-code" value={code} onChange={event => setCode(event.target.value)} /></label>}
    <button disabled={busy || !password || ((!!enrollment || mfaEnabled) && !code)} onClick={() => void mfaAction(mfaEnabled ? 'disable' : enrollment ? 'enable' : 'begin')}>{mfaEnabled ? 'Disable MFA' : enrollment ? 'Confirm authenticator' : 'Set up MFA'}</button>
    {recovery.length > 0 && <section><h3>Save these recovery codes now</h3><p>Each code works once. These codes will disappear when you leave this page.</p><pre>{recovery.join('\n')}</pre><button onClick={() => { const link = document.createElement('a'); link.href = URL.createObjectURL(new Blob([recovery.join('\n')], { type: 'text/plain' })); link.download = 'kari-recovery-codes.txt'; link.click(); URL.revokeObjectURL(link.href) }}>Download recovery codes</button><button onClick={() => setRecovery([])}>I saved my codes</button></section>}
    <p>Email {user.email_verified ? 'verified' : 'not verified'}</p>
    {enabled === null && !error && <p role="status">Checking email availability…</p>}
    {enabled === false && <p>Email delivery is not configured. Contact your administrator to enable verification and account recovery.</p>}
    {message && <p role="status">{message}</p>}{error && <p role="alert">{error}</p>}
    {!user.email_verified && <button disabled={!enabled || busy} onClick={() => void request(true)}>Send verification email</button>}
    <button disabled={!enabled || !user.email_verified || busy} onClick={() => void request(false)}>Send password reset email</button>
  </section>
}
