const memoryStorage = new Map<string, string>()

export const safeStorage = {
  getItem(key: string): string {
    try {
      if (typeof window === 'undefined') return ''
      const storage = window.localStorage
      return storage?.getItem(key) || memoryStorage.get(key) || ''
    } catch {
      return memoryStorage.get(key) || ''
    }
  },

  setItem(key: string, value: string): void {
    memoryStorage.set(key, value)
    try {
      if (typeof window === 'undefined') return
      window.localStorage?.setItem(key, value)
    } catch {
      // Storage may be unavailable in private or embedded browser contexts.
    }
  },

  removeItem(key: string): void {
    memoryStorage.delete(key)
    try {
      if (typeof window === 'undefined') return
      window.localStorage?.removeItem(key)
    } catch {
      // Storage may be unavailable in private or embedded browser contexts.
    }
  },
}
