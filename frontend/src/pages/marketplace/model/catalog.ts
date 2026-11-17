export type CatalogItem = {
  id: string
  name: string
  blurb: string
  price: number
  tone: 'mint' | 'sky' | 'sand' | 'coral'
}

export const CATALOG: CatalogItem[] = [
  {
    id: 'sticker-pack',
    name: 'Neon Sticker Pack',
    blurb: 'Animated chat stickers for your profile.',
    price: 25,
    tone: 'mint',
  },
  {
    id: 'boost-24h',
    name: '24h Boost',
    blurb: 'Pin your posts to the top for a day.',
    price: 50,
    tone: 'sky',
  },
  {
    id: 'theme-aurora',
    name: 'Aurora Theme',
    blurb: 'Soft gradient skins for the player.',
    price: 80,
    tone: 'sand',
  },
  {
    id: 'avatar-frame',
    name: 'Gold Avatar Frame',
    blurb: 'A glowing frame around your avatar.',
    price: 120,
    tone: 'coral',
  },
  {
    id: 'extra-slot',
    name: 'Watchlist Slot',
    blurb: 'Unlock one extra watchlist shelf.',
    price: 40,
    tone: 'mint',
  },
  {
    id: 'emoji-react',
    name: 'Premium Reactions',
    blurb: 'Unlock exclusive emoji reactions.',
    price: 15,
    tone: 'sky',
  },
]
