import { Navigate, NavLink, Route, Routes } from 'react-router-dom'
import './App.css'
import { DashboardPage } from './pages/DashboardPage'
import { DeploymentsPage } from './pages/DeploymentsPage'
import { LoginPage } from './pages/LoginPage'
import { SettingsPage } from './pages/SettingsPage'

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
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/deployments" element={<DeploymentsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
