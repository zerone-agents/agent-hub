interface StatusBadgeProps {
  enabled: boolean
  activeLabel?: string
  inactiveLabel?: string
}

export default function StatusBadge({
  enabled,
  activeLabel = '启用',
  inactiveLabel = '停用'
}: StatusBadgeProps) {
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
      {enabled ? activeLabel : inactiveLabel}
    </span>
  )
}
