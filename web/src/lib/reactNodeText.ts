import { Children, isValidElement, type ReactNode } from 'react'

export function reactNodeText(node: ReactNode): string {
  return Children.toArray(node)
    .map((child) => {
      if (typeof child === 'string' || typeof child === 'number') return String(child)
      if (isValidElement<{ children?: ReactNode }>(child)) return reactNodeText(child.props.children)
      return ''
    })
    .join('')
}
