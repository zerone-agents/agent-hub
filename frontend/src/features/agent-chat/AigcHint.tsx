import { createStyles } from 'antd-style'
import { useTranslation } from 'react-i18next'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  hint: css`
    text-align: center;
    font-size: 12px;
    color: ${tk.textTertiary};
    padding: 4px 0 8px;
    flex-shrink: 0;
    user-select: none;
  `
}))

// GB 45438-2025 显式标识：常驻聊天区域底部，不依赖流状态。
export default function AigcHint() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  return <div className={styles.hint}>{t('agentChat.aigcHint')}</div>
}
