import { Link, Route, Routes } from 'react-router-dom'

function HomePage() {
  return <h1>Home</h1>
}

function LoginPage() {
  return <h1>Login</h1>
}

function SettingsPage() {
  return <h1>Settings</h1>
}

function DeploymentsPage() {
  return <h1>Deployments</h1>
}

function App() {
  return (
    <>
      <nav>
        <ul>
          <li>
            <Link to="/">Home</Link>
          </li>
          <li>
            <Link to="/login">Login</Link>
          </li>
          <li>
            <Link to="/settings">Settings</Link>
          </li>
          <li>
            <Link to="/deployments">Deployments</Link>
          </li>
        </ul>
      </nav>
      <main>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/deployments" element={<DeploymentsPage />} />
        </Routes>
      </main>
    </>
  )
}

export default App
