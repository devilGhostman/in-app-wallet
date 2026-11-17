import type { LedgerItem } from '@/entities/ledger/model'
import { formatDateTime, formatReason, truncateId } from '@/shared/lib/format'

type Props = {
  items: LedgerItem[]
  showOrderSnippet?: boolean
}

export function TransactionList({ items, showOrderSnippet = false }: Props) {
  return (
    <ul className="tx-list">
      {items.map((tx, i) => (
        <li key={tx.ledger_id} className="tx-row" style={{ animationDelay: `${i * 30}ms` }}>
          <div>
            <p className="tx-row__title">{formatReason(tx.reason)}</p>
            <p className="tx-row__meta">
              {formatDateTime(tx.created_at)}
              {showOrderSnippet && (
                <>
                  <span className="tx-row__sep">·</span>
                  bal {tx.balance_after}
                  <span className="tx-row__sep">·</span>
                  {truncateId(tx.order_id)}
                </>
              )}
            </p>
          </div>
          <p className={`tx-row__amt tx-row__amt--${tx.entry_type.toLowerCase()}`}>
            {tx.entry_type === 'CREDIT' ? '+' : '−'}
            {tx.amount}
          </p>
        </li>
      ))}
    </ul>
  )
}
