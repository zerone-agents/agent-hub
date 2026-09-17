import { Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useTranslation } from 'react-i18next'

const useStyles = createStyles(({ css }) => ({
  wrapper: css`
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    width: 100%;
  `
}))

export default function LoadingState({ tip }: { tip?: string }) {
  const { styles } = useStyles()
  const { t } = useTranslation()
  return (
    <div className={styles.wrapper}>
      <Spin size="large" description={tip ?? t('common.loading')} />
    </div>
  )
}
