import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  grid: css`
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 16px;
    // Grid items default to min-width:auto, so long unbreakable content
    // inside a card (chips, URLs) can force the track wider than the
    // viewport on mobile. Allow shrinking to keep one card per row.
    > * {
      min-width: 0;
    }
    @media (max-width: 768px) {
      grid-template-columns: minmax(0, 1fr);
    }
  `
}))

export default function CardGrid({ children }: { children: React.ReactNode }) {
  const { styles } = useStyles()
  return <div className={styles.grid}>{children}</div>
}
