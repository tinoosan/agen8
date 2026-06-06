import * as React from "react"

const MOBILE_BREAKPOINT = 768

export function useIsBelow(maxWidth: number) {
  const [isBelow, setIsBelow] = React.useState<boolean>(() =>
    typeof window !== "undefined" ? window.innerWidth < maxWidth : false
  )

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${maxWidth - 1}px)`)
    const onChange = () => setIsBelow(window.innerWidth < maxWidth)
    mql.addEventListener("change", onChange)
    onChange()
    return () => mql.removeEventListener("change", onChange)
  }, [maxWidth])

  return isBelow
}

export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined)

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    }
    mql.addEventListener("change", onChange)
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    return () => mql.removeEventListener("change", onChange)
  }, [])

  return !!isMobile
}
