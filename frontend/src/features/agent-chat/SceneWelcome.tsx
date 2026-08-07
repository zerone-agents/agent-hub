import { Spin, Tooltip } from 'antd'
import { createStyles } from 'antd-style'
import type { Scene } from '@/api/scenes'
import { useAgentScenes } from '@/queries/useScenes'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 40px 24px;
    gap: 24px;
  `,
  loadingWrap: css`
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
  `,
  hint: css`
    color: ${t.textMuted};
    font-size: 14px;
  `,
  head: css`
    color: ${t.textSecondary};
    font-size: 14px;
    font-weight: 500;
    text-align: center;
  `,
  grid: css`
    width: 100%;
    max-width: 720px;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 12px;
  `,
  card: css`
    padding: 16px;
    border: 1px solid ${t.inkLighter};
    border-radius: ${t.radius}px;
    background: ${t.surface};
    cursor: pointer;
    transition: all 0.15s ease;
    text-align: left;
    &:hover {
      border-color: color-mix(in srgb, var(--foreground) 30%, transparent);
      box-shadow: ${t.elevation1};
      transform: translateY(-1px);
    }
    &:active {
      transform: scale(0.98);
    }
  `,
  cardDisabled: css`
    opacity: 0.5;
    pointer-events: none;
  `,
  cardTitle: css`
    font-weight: 600;
    font-size: 14px;
    color: ${t.text};
    margin-bottom: 6px;
  `,
  cardPreview: css`
    font-size: 12px;
    color: ${t.textTertiary};
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  `,
  footerHint: css`
    color: ${t.textMuted};
    font-size: 12px;
  `
}))

interface SceneWelcomeProps {
  agentName: string
  onPick: (scene: Scene) => void
  disabled?: boolean
}

export default function SceneWelcome({ agentName, onPick, disabled }: SceneWelcomeProps) {
  const { styles } = useStyles()
  const { data: scenes, isLoading } = useAgentScenes(agentName)

  if (isLoading) {
    return (
      <div className={styles.loadingWrap}>
        <Spin />
      </div>
    )
  }

  if (!scenes || scenes.length === 0) {
    return (
      <div className={styles.wrap}>
        <div className={styles.hint}>直接输入消息开始对话</div>
      </div>
    )
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>你可以试试以下场景：</div>
      <div className={styles.grid}>
        {scenes.map((scene) => (
          <Tooltip key={scene.id} title={scene.prompt} placement="top">
            <button
              type="button"
              className={`${styles.card}${disabled ? ` ${styles.cardDisabled}` : ''}`}
              onClick={() => !disabled && onPick(scene)}
            >
              <div className={styles.cardTitle}>{scene.title}</div>
              <div className={styles.cardPreview}>{scene.prompt}</div>
            </button>
          </Tooltip>
        ))}
      </div>
      <div className={styles.footerHint}>（也可直接在下方输入）</div>
    </div>
  )
}
