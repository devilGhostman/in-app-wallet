import { NavLink } from 'react-router-dom'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { PRIMARY_NAV } from '@/shared/constants/nav'
import { formatCoins } from '@/shared/lib/format'
import { shortUserId } from '@/shared/lib/user-id'

export function SiteHeader() {
  const { userId, wallet, loading } = useWallet()

  return (
    <header className="site-header">
      <div className="site-header__inner">
        <NavLink to="/" className="site-header__brand" end>
          CoinWallet
        </NavLink>
        <nav className="site-header__nav" aria-label="Primary">
          {PRIMARY_NAV.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              end={link.end}
              className={({ isActive }) => `site-header__link${isActive ? ' is-active' : ''}`}
            >
              {link.label}
            </NavLink>
          ))}
        </nav>
        <div className="site-header__meta">
          <span className="site-header__balance" title="Available balance">
            {loading && !wallet ? '—' : formatCoins(wallet?.balance ?? 0)}
            <small>coins</small>
          </span>
          <span className="user-chip" title={userId}>
            {shortUserId(userId)}
          </span>
        </div>
      </div>
    </header>
  )
}
