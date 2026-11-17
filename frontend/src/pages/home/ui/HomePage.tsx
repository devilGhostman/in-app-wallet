import { useState } from 'react'
import { Link } from 'react-router-dom'
import { GatewaySheet } from '@/features/top-up/ui/GatewaySheet'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { formatCoins } from '@/shared/lib/format'
import { TransactionList } from '@/widgets/transaction-list/TransactionList'

export function HomePage() {
  const { wallet, recent, loading, error, refresh } = useWallet()
  const [gatewayOpen, setGatewayOpen] = useState(false)

  return (
    <div className="page page--home">
      <div className="home-grid">
        <section className="hero">
          <div className="hero__glow" aria-hidden />
          <p className="brand">CoinWallet</p>
          <p className="hero__tagline">In-app coins for your demo universe.</p>
          <p className="hero__label">Available balance</p>
          <h1 className="hero__balance" data-loading={loading}>
            {loading && !wallet ? '—' : formatCoins(wallet?.balance ?? 0)}
            <span>coins</span>
          </h1>
          <div className="hero__actions">
            <button type="button" className="btn btn--primary" onClick={() => setGatewayOpen(true)}>
              Add tokens
            </button>
            <Link to="/market" className="btn btn--ghost">
              Browse marketplace
            </Link>
          </div>
        </section>

        <section className="panel home-panel">
          <div className="panel__head">
            <h2>Recent activity</h2>
            <Link to="/transactions">See all</Link>
          </div>
          {error && (
            <p className="form-error">
              {error}{' '}
              <button type="button" className="linkish" onClick={() => void refresh()}>
                Retry
              </button>
            </p>
          )}
          {!error && recent.length === 0 && !loading && (
            <p className="empty">No movements yet. Add tokens to get started.</p>
          )}
          <TransactionList items={recent} />
        </section>
      </div>

      <GatewaySheet open={gatewayOpen} onClose={() => setGatewayOpen(false)} />
    </div>
  )
}
