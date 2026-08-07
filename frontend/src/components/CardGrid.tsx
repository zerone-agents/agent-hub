import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  grid: css`
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 16px;
    @media (max-width: 768px) {
      grid-template-columns: 1fr;
    }
  `
}))

export default function CardGrid({ children }: { children: React.ReactNode }) {
  const { styles } = useStyles()
  return <div className={styles.grid}>{children}</div>
}
