import { Navigate, NavLink, Route, Routes } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { ApiError, apiGet, apiPost } from './lib/api'
import type { AccountUser } from './lib/api'
import { AccountPage } from './pages/AccountPage'
import { EmailLinkPage } from './pages/EmailLinkPage'
import { SetupPage } from './pages/SetupPage'
import './App.css'
import { AuditPage } from './pages/AuditPage'
import { DashboardPage } from './pages/DashboardPage'
import { SitesPage } from './pages/SitesPage'
import { WebsitePage } from './pages/WebsitePage'
import { LoginPage } from './pages/LoginPage'
import { RolesPage } from './pages/RolesPage'
import { SystemPage } from './pages/SystemPage'

function App() {
  const [session, setSession] = useState<{ configured: boolean; user: AccountUser | null } | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const status = await apiGet<{ configured: boolean }>('/api/v1/setup/status')
        let user = null
        if (status.configured) {
          try { user = (await apiGet<{ user: AccountUser }>('/api/v1/auth/me')).user }
          catch (error) { if (!(error instanceof ApiError && error.status === 401)) throw error }
        }
        if (!cancelled) { setSession({ ...status, user }); setError('') }
      } catch { if (!cancelled) setError('Cannot connect to Kari. Check the service and try again.') }
    }
    void load()
    const expired = () => setSession(previous => previous ? { ...previous, user: null } : previous)
    window.addEventListener('kari-session-expired', expired)
    return () => { cancelled = true; window.removeEventListener('kari-session-expired', expired) }
  }, [attempt])
  if (error) return <main className="auth-page"><div role="alert">{error}<button onClick={() => { setError(''); setAttempt(attempt + 1) }}>Try again</button></div></main>
  if (!session) return <main className="auth-page" role="status">Connecting to Kari…</main>
  if (!session.configured) return <Routes>
    <Route path="/setup" element={<SetupPage onComplete={() => setSession({ configured: true, user: null })} />} />
    <Route path="*" element={<Navigate to="/setup" replace />} />
  </Routes>
  return (
    <Routes>
      <Route path="/auth/email/:purpose" element={<EmailLinkPage onSignedIn={user => setSession({ configured: true, user })} onReset={() => setSession({ configured: true, user: null })} />} />
      <Route path="/setup" element={<Navigate to={session.user ? '/' : '/login?setup=complete'} replace />} />
      <Route path="/login" element={session.user ? <Navigate to="/" replace /> : <LoginPage onSignedIn={user => setSession({ configured: true, user })} />} />
      <Route path="*" element={session.user ? <AppShell user={session.user} onUserChange={user => setSession({ configured:true,user })} onSignedOut={() => setSession({ configured: true, user: null })} /> : <Navigate to="/login" replace />} />
    </Routes>
  )
}

function AppShell({ user, onSignedOut, onUserChange }: { user: AccountUser; onSignedOut: () => void; onUserChange: (user: AccountUser) => void }) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function logout() {
    setBusy(true)
    try { await apiPost('/api/v1/auth/logout', {}); onSignedOut() }
    catch { setError('Could not sign out. Please try again.') }
    finally { setBusy(false) }
  }
  return (
    <div className="app-shell">
      <header className="top-nav">
        <div className="brand">Karı</div>
        <nav>
          <ul className="nav-list">
            <li><NavLink to="/account" className="nav-link">Account</NavLink></li>
            <li>
              <NavLink to="/" end className="nav-link">
                Dashboard
              </NavLink>
            </li>
            <li>
              <NavLink to="/sites" className="nav-link">
                Websites
              </NavLink>
            </li>
            <li>
              <NavLink to="/roles" className="nav-link">
                Roles
              </NavLink>
            </li>
            <li>
              <NavLink to="/audit" className="nav-link">
                Audit
              </NavLink>
            </li>
            <li>
              <NavLink to="/system" className="nav-link">
                System
              </NavLink>
            </li>
            <li>
              <NavLink to="/settings" className="nav-link">
                Server settings
              </NavLink>
            </li>
          </ul>
        </nav>
        <span>{user.email}</span>
        <button disabled={busy} onClick={() => void logout()}>Sign out</button>
      </header>
      <main className="page-main">
        {error && <p role="alert">{error}</p>}
        <Routes>
          <Route path="/account" element={<AccountPage user={user} onUserChange={onUserChange} />} />
          <Route path="/" element={user.mfa_required && !user.mfa_enabled ? <Navigate to="/account" replace /> : <DashboardPage />} />
          <Route path="/sites" element={<SitesPage />} />
          <Route path="/sites/:id" element={<WebsitePage />} />
          <Route path="/deployments" element={<Navigate to="/sites" replace />} />
          <Route path="/domains" element={<Navigate to="/sites" replace />} />
          <Route path="/roles" element={<RolesPage />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="/system" element={<SystemPage />} />
          <Route path="/settings" element={<Navigate to="/system" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
