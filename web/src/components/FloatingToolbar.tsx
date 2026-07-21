import type { ReactNode } from 'react'
import { Tooltip } from 'antd'

export interface FloatingToolbarItem {
  key: string
  label: string
  icon: ReactNode
  onClick: () => void
  disabled?: boolean
  loading?: boolean
  danger?: boolean
  active?: boolean
}

interface Props {
  ariaLabel: string
  items: FloatingToolbarItem[]
}

export function FloatingToolbar({ ariaLabel, items }: Props) {
  return <div aria-label={ariaLabel} className="floating-toolbar" role="toolbar">
    {items.map((item) => <Tooltip key={item.key} placement="left" title={item.label}>
      <button aria-label={item.label} aria-pressed={item.active} className={`floating-toolbar-button${item.danger ? ' danger' : ''}${item.active ? ' active' : ''}`} disabled={item.disabled || item.loading} onClick={item.onClick} type="button">
        <span className={`floating-toolbar-icon${item.loading ? ' loading' : ''}`}>{item.icon}</span>
      </button>
    </Tooltip>)}
  </div>
}
