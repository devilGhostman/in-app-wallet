import { useWallet } from '@/features/wallet-session/WalletProvider'

export function Toast() {
  const { toast, clearToast } = useWallet()
  if (!toast) return null
  return (
    <div className="toast" role="status" onClick={clearToast}>
      {toast}
    </div>
  )
}
