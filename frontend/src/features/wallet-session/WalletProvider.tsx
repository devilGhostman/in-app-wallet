import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { listLedger } from '@/entities/ledger/api'
import type { LedgerItem } from '@/entities/ledger/model'
import { ensureWallet } from '@/entities/wallet/api'
import type { Wallet } from '@/entities/wallet/model'
import { toApiError, type ApiError } from '@/shared/api/errors'
import { getOrCreateUserId } from '@/shared/lib/user-id'

type WalletSession = {
  userId: string
  wallet: Wallet | null
  recent: LedgerItem[]
  loading: boolean
  error: string | null
  toast: string | null
  refresh: () => Promise<void>
  showToast: (message: string) => void
  clearToast: () => void
}

const WalletSessionContext = createContext<WalletSession | null>(null)

export function WalletProvider({ children }: { children: ReactNode }) {
  const userId = useMemo(() => getOrCreateUserId(), [])
  const [wallet, setWallet] = useState<Wallet | null>(null)
  const [recent, setRecent] = useState<LedgerItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setError(null)
    try {
      const nextWallet = await ensureWallet(userId)
      const page = await listLedger(userId, { limit: 8 })
      setWallet(nextWallet)
      setRecent(page.transactions ?? [])
    } catch (e) {
      const err = toApiError(e) as ApiError
      setError(err.message || 'Failed to load wallet')
    } finally {
      setLoading(false)
    }
  }, [userId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const showToast = useCallback((message: string) => {
    setToast(message)
  }, [])

  const clearToast = useCallback(() => setToast(null), [])

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 3200)
    return () => window.clearTimeout(timer)
  }, [toast])

  const value = useMemo(
    () => ({
      userId,
      wallet,
      recent,
      loading,
      error,
      toast,
      refresh,
      showToast,
      clearToast,
    }),
    [userId, wallet, recent, loading, error, toast, refresh, showToast, clearToast],
  )

  return <WalletSessionContext.Provider value={value}>{children}</WalletSessionContext.Provider>
}

export function useWallet(): WalletSession {
  const ctx = useContext(WalletSessionContext)
  if (!ctx) {
    throw new Error('useWallet must be used within WalletProvider')
  }
  return ctx
}
