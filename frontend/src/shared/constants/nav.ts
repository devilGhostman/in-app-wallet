export type NavItem = {
  to: string
  label: string
  shortLabel?: string
  end?: boolean
}

export const PRIMARY_NAV: NavItem[] = [
  { to: '/', label: 'Home', end: true },
  { to: '/market', label: 'Marketplace', shortLabel: 'Market' },
  { to: '/transactions', label: 'Activity' },
]
