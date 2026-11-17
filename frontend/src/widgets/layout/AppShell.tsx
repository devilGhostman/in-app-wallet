import type { ReactNode } from 'react'
import { Toast } from '@/shared/ui/Toast'
import { BottomNav } from './BottomNav'
import { SiteHeader } from './SiteHeader'

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <SiteHeader />
      <main className="app-main">{children}</main>
      <BottomNav />
      <Toast />
    </div>
  )
}
