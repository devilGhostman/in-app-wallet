import { useState } from 'react'
import {
  createPurchaseOrder,
  pollOrderTerminal,
  simulatePayment,
} from '@/entities/order/api'
import { useWallet } from '@/features/wallet-session/WalletProvider'
import { toApiError } from '@/shared/api/errors'
import { newIdempotencyKey } from '@/shared/lib/idempotency'
import { TOP_UP_AMOUNTS, type TopUpStep } from '../model/constants'

type Props = {
  open: boolean
  onClose: () => void
}

export function GatewaySheet({ open, onClose }: Props) {
  const { userId, refresh, showToast } = useWallet()
  const [step, setStep] = useState<TopUpStep>('amount')
  const [amount, setAmount] = useState<number>(TOP_UP_AMOUNTS[1])
  const [orderId, setOrderId] = useState<string | null>(null)
  const [result, setResult] = useState<'success' | 'fail' | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  if (!open) return null

  const resetAndClose = () => {
    setStep('amount')
    setAmount(TOP_UP_AMOUNTS[1])
    setOrderId(null)
    setResult(null)
    setBusy(false)
    setError(null)
    onClose()
  }

  const startCheckout = async () => {
    setBusy(true)
    setError(null)
    try {
      const order = await createPurchaseOrder({
        userId,
        amount,
        idempotencyKey: newIdempotencyKey('purchase'),
        metadata: { source: 'demo-gateway' },
      })
      setOrderId(order.order_id)
      setStep('pay')
    } catch (e) {
      setError(toApiError(e).message || 'Could not create order')
    } finally {
      setBusy(false)
    }
  }

  const onPaySuccess = async () => {
    if (!orderId) return
    setBusy(true)
    setError(null)
    setStep('processing')
    try {
      await simulatePayment(orderId)
      const final = await pollOrderTerminal(orderId)
      if (final.status === 'completed') {
        setResult('success')
        showToast(`+${amount} coins added`)
        await refresh()
      } else {
        setResult('fail')
        setError(final.failure_message || final.failure_code || 'Credit failed')
      }
      setStep('done')
    } catch (e) {
      setResult('fail')
      setError(toApiError(e).message || 'Payment processing failed')
      setStep('done')
    } finally {
      setBusy(false)
    }
  }

  const onPayFail = () => {
    setResult('fail')
    setError('Payment declined by dummy gateway')
    setStep('done')
  }

  return (
    <div className="sheet-backdrop" onClick={busy ? undefined : resetAndClose}>
      <div
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-label="Add tokens"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sheet__handle" aria-hidden />
        <header className="sheet__header">
          <h2>Add tokens</h2>
          <p>Dummy payment gateway — choose success or fail.</p>
        </header>

        {step === 'amount' && (
          <div className="sheet__body">
            <div className="amount-grid">
              {TOP_UP_AMOUNTS.map((n) => (
                <button
                  key={n}
                  type="button"
                  className={`amount-chip${amount === n ? ' is-selected' : ''}`}
                  onClick={() => setAmount(n)}
                >
                  {n}
                </button>
              ))}
            </div>
            {error && <p className="form-error">{error}</p>}
            <button type="button" className="btn btn--primary" disabled={busy} onClick={() => void startCheckout()}>
              {busy ? 'Creating order…' : `Continue · ${amount} coins`}
            </button>
          </div>
        )}

        {step === 'pay' && (
          <div className="sheet__body gateway-card">
            <p className="gateway-card__label">PaySecure Demo</p>
            <p className="gateway-card__amount">{amount} coins</p>
            <p className="gateway-card__meta">Order {orderId?.slice(0, 8)}…</p>
            <div className="gateway-actions">
              <button type="button" className="btn btn--primary" disabled={busy} onClick={() => void onPaySuccess()}>
                Payment successful
              </button>
              <button type="button" className="btn btn--ghost" disabled={busy} onClick={onPayFail}>
                Payment failed
              </button>
            </div>
          </div>
        )}

        {step === 'processing' && (
          <div className="sheet__body center">
            <div className="spinner" aria-hidden />
            <p>Confirming wallet credit…</p>
          </div>
        )}

        {step === 'done' && (
          <div className="sheet__body center">
            <p className={`result-badge result-badge--${result}`}>
              {result === 'success' ? 'Payment complete' : 'Payment failed'}
            </p>
            {error && <p className="form-error">{error}</p>}
            <button type="button" className="btn btn--primary" onClick={resetAndClose}>
              Done
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
