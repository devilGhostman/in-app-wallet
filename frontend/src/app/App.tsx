import { AppProviders } from '@/app/providers/AppProviders'
import { AppRoutes } from '@/app/router/routes'
import { AppShell } from '@/widgets/layout/AppShell'
import '@/app/styles/app.css'

export default function App() {
  return (
    <AppProviders>
      <AppShell>
        <AppRoutes />
      </AppShell>
    </AppProviders>
  )
}
