import { useCallback, useEffect, useState } from 'react'
import { listLedger } from '@/entities/ledger/api'
import type { LedgerEntryType, LedgerItem } from '@/entities/ledger/model'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { toApiError } from '@/shared/api/errors'
import { TransactionList } from '@/widgets/transaction-list/TransactionList'

export function TransactionsPage() {
  const { userId } = useWallet()
  const [tab, setTab] = useState<LedgerEntryType>('CREDIT')
  const [items, setItems] = useState<LedgerItem[]>([])
  const [cursor, setCursor] = useState<string | undefined>()
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(
    async (entryType: LedgerEntryType, next?: string, append = false) => {
      if (append) setLoadingMore(true)
      else setLoading(true)
      setError(null)
      try {
        const page = await listLedger(userId, {
          entryType,
          limit: 20,
          cursor: next,
        })
        setItems((prev) => (append ? [...prev, ...(page.transactions ?? [])] : page.transactions ?? []))
        setCursor(page.next_cursor || undefined)
      } catch (e) {
        setError(toApiError(e).message || 'Failed to load ledger')
      } finally {
        setLoading(false)
        setLoadingMore(false)
      }
    },
    [userId],
  )

  useEffect(() => {
    void load(tab)
  }, [tab, load])

  return (
    <div className="page page--txns">
      <header className="page-header page-header--spread">
        <div>
          <h1>Activity</h1>
          <p>Credits and debits from your wallet ledger.</p>
        </div>
        <div className="tabs tabs--inline" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'CREDIT'}
            className={tab === 'CREDIT' ? 'is-active' : ''}
            onClick={() => setTab('CREDIT')}
          >
            Credits
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'DEBIT'}
            className={tab === 'DEBIT' ? 'is-active' : ''}
            onClick={() => setTab('DEBIT')}
          >
            Debits
          </button>
        </div>
      </header>

      {error && <p className="form-error">{error}</p>}
      {loading && <p className="empty">Loading…</p>}
      {!loading && items.length === 0 && <p className="empty">No {tab.toLowerCase()}s yet.</p>}

      <div className="tx-table-wrap">
        <TransactionList items={items} showOrderSnippet />
      </div>

      {cursor && (
        <button
          type="button"
          className="btn btn--ghost"
          disabled={loadingMore}
          onClick={() => void load(tab, cursor, true)}
        >
          {loadingMore ? 'Loading…' : 'Load more'}
        </button>
      )}
    </div>
  )
}
