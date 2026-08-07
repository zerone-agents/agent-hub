import { type ReactNode } from 'react'
import { Empty, Button } from 'antd'
import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  wrapper: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 64px 24px;
  `
}))

interface EmptyStateProps {
  description: string
  image?: ReactNode
  action?: {
    label: string
    onClick: () => void
  }
}

export default function EmptyState({ description, image, action }: EmptyStateProps) {
  const { styles } = useStyles()
  return (
    <div className={styles.wrapper}>
      <Empty
        description={description}
        image={image ?? undefined}
      />
      {action && (
        <Button type="primary" style={{ marginTop: 16 }} onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  )
}
