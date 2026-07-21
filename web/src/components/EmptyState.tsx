import { PlusOutlined } from '@ant-design/icons'
import { Button, Empty, Typography } from 'antd'

export function EmptyState({ title, description, actionLabel, onAction }: { title: string; description: string; actionLabel?: string; onAction?: () => void }) {
  return <div className="center-empty"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<div><Typography.Title level={4}>{title}</Typography.Title><Typography.Paragraph type="secondary">{description}</Typography.Paragraph></div>}>{actionLabel && onAction ? <Button icon={<PlusOutlined/>} onClick={onAction} type="primary">{actionLabel}</Button> : null}</Empty></div>
}
