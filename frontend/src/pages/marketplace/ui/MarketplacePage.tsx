import { usePurchaseItem } from '@/features/spend/usePurchaseItem'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { formatCoins } from '@/shared/lib/format'
import { CATALOG } from '../model/catalog'

export function MarketplacePage() {
  const { wallet } = useWallet()
  const { buyingId, error, purchase } = usePurchaseItem()

  return (
    <div className="page page--market">
      <header className="page-header page-header--spread">
        <div>
          <h1>Marketplace</h1>
          <p>Spend coins on dummy perks for your demo profile.</p>
        </div>
        <p className="page-header__stat">
          <strong>{formatCoins(wallet?.balance ?? 0)}</strong>
          <span>coins available</span>
        </p>
      </header>
      {error && <p className="form-error">{error}</p>}
      <ul className="catalog">
        {CATALOG.map((item, i) => (
          <li
            key={item.id}
            className={`catalog-item catalog-item--${item.tone}`}
            style={{ animationDelay: `${i * 45}ms` }}
          >
            <div className="catalog-item__body">
              <span className="catalog-item__tone" aria-hidden />
              <h2>{item.name}</h2>
              <p>{item.blurb}</p>
            </div>
            <button
              type="button"
              className="btn btn--primary btn--compact"
              disabled={buyingId !== null}
              onClick={() => void purchase(item)}
            >
              {buyingId === item.id ? 'Buying…' : `${item.price} coins`}
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
