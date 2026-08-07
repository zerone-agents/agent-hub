import { Spin } from 'antd'
import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  wrapper: css`
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    width: 100%;
  `
}))

export default function LoadingState({ tip = '加载中...' }: { tip?: string }) {
  const { styles } = useStyles()
  return (
    <div className={styles.wrapper}>
      <Spin size="large" description={tip} />
    </div>
  )
}
