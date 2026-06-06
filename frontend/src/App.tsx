import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import RunsList from './views/RunsList'
import NewRun from './views/NewRun'
import LiveRun from './views/LiveRun'
import RunDetail from './views/RunDetail'
import Compare from './views/Compare'

export default function App() {
  return (
    <BrowserRouter>
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
      </nav>
      <Routes>
        <Route path="/" element={<NewRun />} />
        <Route path="/runs" element={<RunsList />} />
        <Route path="/runs/new" element={<NewRun />} />
        <Route path="/runs/:id/live" element={<LiveRun />} />
        <Route path="/runs/:id" element={<RunDetail />} />
        <Route path="/compare" element={<Compare />} />
      </Routes>
    </BrowserRouter>
  )
}
