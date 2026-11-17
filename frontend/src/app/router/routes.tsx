import { Navigate, Route, Routes } from 'react-router-dom'
import { HomePage } from '@/pages/home/ui/HomePage'
import { MarketplacePage } from '@/pages/marketplace/ui/MarketplacePage'
import { TransactionsPage } from '@/pages/transactions/ui/TransactionsPage'

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route path="/market" element={<MarketplacePage />} />
      <Route path="/transactions" element={<TransactionsPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
