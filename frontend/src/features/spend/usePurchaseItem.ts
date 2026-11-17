import { useCallback, useState } from 'react'
import { createSpendOrder, pollOrderTerminal } from '@/entities/order/api'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { toApiError } from '@/shared/api/errors'
import { newIdempotencyKey } from '@/shared/lib/idempotency'

export type PurchasableItem = {
  id: string
  name: string
  price: number
}

export function usePurchaseItem() {
  const { userId, wallet, refresh, showToast } = useWallet()
  const [buyingId, setBuyingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const purchase = useCallback(
    async (item: PurchasableItem) => {
      setError(null)
      const balance = wallet?.balance ?? 0
      if (balance < item.price) {
        setError('Not enough coins — add tokens from Home first.')
        return
      }

      setBuyingId(item.id)
      try {
        const order = await createSpendOrder({
          userId,
          amount: item.price,
          idempotencyKey: newIdempotencyKey(`spend:${item.id}`),
          metadata: { item_id: item.id, item_name: item.name },
        })
        const final = await pollOrderTerminal(order.order_id)
        if (final.status === 'completed') {
          showToast(`Bought ${item.name}`)
          await refresh()
          return
        }
        setError(final.failure_message || final.failure_code || 'Spend failed')
        showToast('Purchase failed')
      } catch (e) {
        setError(toApiError(e).message || 'Could not complete purchase')
      } finally {
        setBuyingId(null)
      }
    },
    [refresh, showToast, userId, wallet?.balance],
  )

  return { buyingId, error, purchase, clearError: () => setError(null) }
}
