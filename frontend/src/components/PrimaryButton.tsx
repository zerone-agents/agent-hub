import type { ButtonProps } from 'antd'
import { Button } from 'antd'
import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  root: css`
    &&& {
      color: var(--primary-foreground) !important;
      svg { color: currentColor; }
    }
    &&&[disabled] {
      background: var(--accent) !important;
      border-color: var(--accent) !important;
    }
  `
}))

export default function PrimaryButton({ className, ...rest }: ButtonProps) {
  const { styles } = useStyles()
  return <Button type="primary" className={`${styles.root} ${className ?? ''}`} {...rest} />
}
