import { NavLink, Outlet } from 'react-router-dom'
import { LayoutDashboard, Bot, ListTodo, Activity, Cpu, Sun, Moon, Users, UserRound } from 'lucide-react'
import { useTheme } from '../hooks/useTheme'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/agents', icon: Bot, label: 'Agents' },
  { to: '/humans', icon: UserRound, label: 'Humans' },
  { to: '/builtin-agents', icon: Cpu, label: 'Builtin Agents' },
  { to: '/groups', icon: Users, label: 'Groups' },
  { to: '/tasks', icon: ListTodo, label: 'Tasks' },
  { to: '/traces', icon: Activity, label: 'Traces' },
]

export default function Layout() {
  const { dark, toggle } = useTheme()

  return (
    <div className="flex h-screen overflow-hidden">
      <aside className="w-56 shrink-0 border-r border-[var(--border)] bg-[var(--bg-secondary)] flex flex-col">
        <div className="p-5 border-b border-[var(--border)]">
          <h1 className="text-base font-semibold tracking-tight text-[var(--text-primary)]">
            A2A Platform
          </h1>
          <p className="text-xs text-[var(--text-tertiary)] mt-0.5">Admin Console</p>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {navItems.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? 'bg-[var(--bg-tertiary)] text-[var(--text-primary)]'
                    : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]/50'
                }`
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="p-3 border-t border-[var(--border)]">
          <button
            onClick={toggle}
            className="flex items-center gap-2.5 w-full px-3 py-2 rounded-md text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]/50 transition-colors"
          >
            {dark ? <Sun size={16} /> : <Moon size={16} />}
            {dark ? 'Light mode' : 'Dark mode'}
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}
