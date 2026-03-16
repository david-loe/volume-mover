import { Link, NavLink } from 'react-router-dom'
import type { ReactNode } from 'react'

const items = [
  { to: '/app/dashboard', label: 'Dashboard' },
  { to: '/app/hosts', label: 'Hosts' },
  { to: '/app/volumes', label: 'Volumes' },
  { to: '/app/transfers/new', label: 'New Transfer' },
  { to: '/app/transfers', label: 'Jobs' },
]

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link className="brand" to="/app/dashboard">
          <span className="brand-kicker">Volume Mover</span>
          <strong>Operations UI</strong>
        </Link>
        <nav className="sidebar-nav">
          {items.map((item) => (
            <NavLink key={item.to} to={item.to} className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="app-main">
        <header className="topbar">
          <div>
            <p className="eyebrow">Docker volume orchestration</p>
            <h1>Control Plane</h1>
          </div>
          <div className="topbar-actions">
            <Link className="button ghost" to="/app/transfers/new">New Transfer</Link>
          </div>
        </header>
        <div className="page-body">{children}</div>
      </main>
    </div>
  )
}
