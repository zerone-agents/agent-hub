import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  card: css`
    background: ${t.surface};
    border-radius: ${t.radius}px;
    border: 1px solid var(--border);
    box-shadow: ${t.elevation1};
    padding: 18px 20px 16px;
    display: flex;
    flex-direction: column;
    transition: box-shadow 0.2s ease, transform 0.2s ease;
    animation: cardUp 0.35s ease backwards;
    &:hover {
      border-color: color-mix(in srgb, var(--primary) 34%, var(--border));
      box-shadow: ${t.elevation2};
      transform: translateY(-1px);
    }
    @keyframes cardUp {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 10px;
  `,
  icon: css`
    width: 38px;
    height: 38px;
    border-radius: ${t.radiusSm}px;
    background: ${t.inkSubtle};
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 15px;
    font-weight: 600;
    color: ${t.ink};
    flex-shrink: 0;
  `,
  titleWrap: css`
    flex: 1;
    min-width: 0;
  `,
  title: css`
    font-size: 14px;
    font-weight: 600;
    color: ${t.text};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  `,
  subtitle: css`
    font-family: ${t.fontMono};
    font-size: 11px;
    color: ${t.textMuted};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  `,
  headerExtra: css`
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    justify-content: flex-end;
  `,
  description: css`
    font-size: 12px;
    color: ${t.textTertiary};
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    min-height: 3em;
  `,
  bodyExtra: css`
    margin-top: 8px;
    flex-grow: 1;
  `,
  footer: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-top: 14px;
    margin-top: 14px;
    border-top: 1px solid var(--border);
  `,
  footerLeft: css`
    font-size: 11px;
    color: ${t.textMuted};
  `,
  footerRight: css`
    display: flex;
    gap: 2px;
  `
}))

export interface EntityCardProps {
  icon?: React.ReactNode
  title: React.ReactNode
  subtitle?: string
  headerExtra?: React.ReactNode
  description?: string
  bodyExtra?: React.ReactNode
  footerLeft?: React.ReactNode
  footerRight?: React.ReactNode
}

export default function EntityCard({
  icon,
  title,
  subtitle,
  headerExtra,
  description,
  bodyExtra,
  footerLeft,
  footerRight
}: EntityCardProps) {
  const { styles } = useStyles()
  const firstLetter = typeof title === 'string' ? title.charAt(0).toUpperCase() : ''

  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <div className={styles.icon}>{icon || firstLetter}</div>
        <div className={styles.titleWrap}>
          <div className={styles.title}>{title}</div>
          {subtitle && <div className={styles.subtitle}>{subtitle}</div>}
        </div>
        {headerExtra && <div className={styles.headerExtra}>{headerExtra}</div>}
      </div>
      {description && <div className={styles.description}>{description}</div>}
      {bodyExtra && <div className={styles.bodyExtra}>{bodyExtra}</div>}
      {(footerLeft || footerRight) && (
        <div className={styles.footer}>
          {footerLeft && <div className={styles.footerLeft}>{footerLeft}</div>}
          {footerRight && <div className={styles.footerRight}>{footerRight}</div>}
        </div>
      )}
    </div>
  )
}
