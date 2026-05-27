import { useState, useEffect } from 'react'
import { safeStorage } from '../utils/storage'

export function useTheme() {
  const [dark, setDark] = useState(() => {
    const saved = safeStorage.getItem('theme')
    if (saved) return saved === 'dark'
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  })

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    safeStorage.setItem('theme', dark ? 'dark' : 'light')
  }, [dark])

  return { dark, toggle: () => setDark(d => !d) }
}
