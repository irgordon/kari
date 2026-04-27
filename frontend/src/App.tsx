import { Navigate, NavLink, Route, Routes } from 'react-router-dom'
import './App.css'
import { AuditPage } from './pages/AuditPage'
import { DashboardPage } from './pages/DashboardPage'
import { DeploymentsPage } from './pages/DeploymentsPage'
import { DomainsPage } from './pages/DomainsPage'
import { LoginPage } from './pages/LoginPage'
import { RolesPage } from './pages/RolesPage'
import { SettingsPage } from './pages/SettingsPage'
import { SystemPage } from './pages/SystemPage'

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="*" element={<AppShell />} />
    </Routes>
  )
}

function AppShell() {
  return (
    <div className="app-shell">
      <header className="top-nav">
        <div className="brand">Karı</div>
        <nav>
          <ul className="nav-list">
            <li>
              <NavLink to="/" end className="nav-link">
                Dashboard
              </NavLink>
            </li>
            <li>
              <NavLink to="/deployments" className="nav-link">
                Deployments
              </NavLink>
            </li>
            <li>
              <NavLink to="/domains" className="nav-link">
                Domains
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
                Settings
              </NavLink>
            </li>
          </ul>
        </nav>
      </header>
      <main className="page-main">
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/deployments" element={<DeploymentsPage />} />
          <Route path="/domains" element={<DomainsPage />} />
          <Route path="/roles" element={<RolesPage />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="/system" element={<SystemPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
