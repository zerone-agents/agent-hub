import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  hint: css`
    text-align: center;
    font-size: 12px;
    color: ${t.textTertiary};
    padding: 4px 0 8px;
    flex-shrink: 0;
    user-select: none;
  `
}))

// GB 45438-2025 显式标识：常驻聊天区域底部，不依赖流状态。
export default function AigcHint() {
  const { styles } = useStyles()
  return <div className={styles.hint}>内容由 AI 生成，请仔细甄别</div>
}
