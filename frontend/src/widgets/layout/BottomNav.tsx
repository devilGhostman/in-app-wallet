import { NavLink } from 'react-router-dom'
import { PRIMARY_NAV } from '@/shared/constants/nav'

export function BottomNav() {
  return (
    <nav className="bottom-nav" aria-label="Primary">
      {PRIMARY_NAV.map((link) => (
        <NavLink
          key={link.to}
          to={link.to}
          end={link.end}
          className={({ isActive }) => `bottom-nav__link${isActive ? ' is-active' : ''}`}
        >
          <span className="bottom-nav__dot" aria-hidden />
          {link.shortLabel ?? link.label}
        </NavLink>
      ))}
    </nav>
  )
}
