import * as React from "react"

type Theme = "dark" | "light" | "system"

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  theme: Theme
  setTheme: (theme: Theme) => void
}

const initialState: ThemeProviderState = {
  theme: "system",
  setTheme: () => null,
}

const ThemeProviderContext = React.createContext<ThemeProviderState>(initialState)

export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "moopicview-ui-theme",
  ...props
}: ThemeProviderProps) {
  const [theme, setTheme] = React.useState<Theme>(
    () => {
      const savedTheme = localStorage.getItem(storageKey) as Theme
      const initialTheme = savedTheme || defaultTheme
      console.log(`[Theme] Initial theme: ${initialTheme} (from ${savedTheme ? 'localStorage' : 'default'})`)
      return initialTheme
    }
  )

  React.useEffect(() => {
    const root = window.document.documentElement
    root.classList.remove("light", "dark")

    if (theme === "system") {
      const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
        .matches
        ? "dark"
        : "light"
      root.classList.add(systemTheme)
      console.log(`[Theme] System preference applied: ${systemTheme}`)
    } else {
      root.classList.add(theme)
      console.log(`[Theme] Manual theme applied: ${theme}`)
    }
  }, [theme])

  // Listen for system theme changes
  React.useEffect(() => {
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    const handleChange = (e: MediaQueryListEvent) => {
      if (theme === "system") {
        const systemTheme = e.matches ? "dark" : "light"
        const root = window.document.documentElement
        root.classList.remove("light", "dark")
        root.classList.add(systemTheme)
        console.log(`[Theme] System theme changed to: ${systemTheme}`)
      }
    }

    mediaQuery.addEventListener("change", handleChange)
    return () => mediaQuery.removeEventListener("change", handleChange)
  }, [theme])

  const value = {
    theme,
    setTheme: (theme: Theme) => {
      localStorage.setItem(storageKey, theme)
      setTheme(theme)
      console.log(`[Theme] User selected: ${theme}`)
    },
  }

  return (
    <ThemeProviderContext.Provider {...props} value={value}>
      {children}
    </ThemeProviderContext.Provider>
  )
}

export const useTheme = () => {
  const context = React.useContext(ThemeProviderContext)

  if (context === undefined)
    throw new Error("useTheme must be used within a ThemeProvider")

  return context
}
