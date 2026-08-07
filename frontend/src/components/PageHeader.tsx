import { type ReactNode } from 'react'
import { Flex, Typography } from 'antd'
import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css, token }) => ({
  header: css`
    margin-bottom: ${token.marginLG}px;
  `,
  title: css`
    font-size: ${token.fontSizeHeading3}px;
    font-weight: 600;
    color: ${token.colorText};
    line-height: 1.3;
  `,
  extra: css`
    margin-left: auto;
  `
}))

interface PageHeaderProps {
  title: string
  subtitle?: string
  extra?: ReactNode
}

export default function PageHeader({ title, subtitle, extra }: PageHeaderProps) {
  const { styles } = useStyles()
  return (
    <Flex align="center" className={styles.header}>
      <div>
        <div className={styles.title}>{title}</div>
        {subtitle && <Typography.Text type="secondary">{subtitle}</Typography.Text>}
      </div>
      {extra && <div className={styles.extra}>{extra}</div>}
    </Flex>
  )
}
