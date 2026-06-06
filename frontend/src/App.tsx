import { BrowserRouter, Routes, Route, NavLink, Outlet, Navigate, useNavigate } from 'react-router-dom'
import RunsList from './views/RunsList'
import NewRun from './views/NewRun'
import LiveRun from './views/LiveRun'
import RunDetail from './views/RunDetail'
import Compare from './views/Compare'
import Login from './views/Login'
import { isAuthenticated, clearToken } from './auth'

function AuthGuard() {
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}

function NavBar() {
  const navigate = useNavigate()

  function handleSignOut() {
    clearToken()
    navigate('/login')
  }

  return (
    <nav className="nav">
      <span className="nav-brand">prompt<span>evo</span></span>
      <div className="nav-links">
        <NavLink to="/" end className={({ isActive }) => 'nav-link' + (isActive ? ' active' : '')}>
          Launch
        </NavLink>
        <NavLink to="/runs" className={({ isActive }) => 'nav-link' + (isActive ? ' active' : '')}>
          Runs
        </NavLink>
        <NavLink to="/compare" className={({ isActive }) => 'nav-link' + (isActive ? ' active' : '')}>
          Compare
        </NavLink>
      </div>
      <button
        onClick={handleSignOut}
        className="btn btn-secondary btn-sm"
        style={{ marginLeft: 'auto' }}
      >
        Sign out
      </button>
    </nav>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<AuthGuard />}>
          <Route
            element={
              <>
                <NavBar />
                <Outlet />
              </>
            }
          >
            <Route path="/" element={<NewRun />} />
            <Route path="/runs" element={<RunsList />} />
            <Route path="/runs/new" element={<NewRun />} />
            <Route path="/runs/:id/live" element={<LiveRun />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/compare" element={<Compare />} />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
