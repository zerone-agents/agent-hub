import { useTranslation } from 'react-i18next'

interface StatusBadgeProps {
  enabled: boolean
  activeLabel?: string
  inactiveLabel?: string
}

// 默认值走 t() 的哨兵模式（参数默认值作用域拿不到 hook 的 t）。
export default function StatusBadge({
  enabled,
  activeLabel,
  inactiveLabel
}: StatusBadgeProps) {
  const { t } = useTranslation()
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '1px 7px',
        borderRadius: '3px',
        fontSize: '10px',
        fontWeight: 600,
        letterSpacing: '0.02em',
        textTransform: 'uppercase',
        background: enabled ? 'rgba(5, 150, 105, 0.08)' : 'rgba(107, 114, 128, 0.08)',
        color: enabled ? 'var(--success)' : 'var(--text-muted)',
      }}
    >
      {enabled
        ? (activeLabel ?? t('components.statusBadge.active'))
        : (inactiveLabel ?? t('components.statusBadge.inactive'))}
    </span>
  )
}
