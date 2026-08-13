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

/**
 * 主色填充按钮的样式 hook。
 *
 * 用法：
 * - 独立按钮：直接用 `<PrimaryButton>`。
 * - Modal OK 按钮：antd Modal 的 OK 按钮内部已带 `type="primary"`，
 *   只需通过 `okButtonProps={{ className: styles.root }}` 注入相同样式，
 *   无需把整个 footer 改成自定义按钮。
 */
export function usePrimaryButtonStyle() {
  const { styles } = useStyles()
  return styles
}

export default function PrimaryButton({ className, ...rest }: ButtonProps) {
  const styles = usePrimaryButtonStyle()
  return <Button type="primary" className={`${styles.root} ${className ?? ''}`} {...rest} />
}
