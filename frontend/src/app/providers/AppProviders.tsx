import type { ReactNode } from 'react'
import { BrowserRouter } from 'react-router-dom'
import { WalletProvider } from '@/features/wallet-session/WalletProvider'

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <WalletProvider>
      <BrowserRouter>{children}</BrowserRouter>
    </WalletProvider>
  )
}
