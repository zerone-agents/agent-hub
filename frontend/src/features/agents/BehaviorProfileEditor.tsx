import { Slider } from 'antd'
import { createStyles } from 'antd-style'
import type { BehaviorProfile } from '@/api/agents'
import {
  BEHAVIOR_PRESETS,
  BEHAVIOR_TRAITS,
  cloneBehaviorProfile,
  profilesEqual,
} from './behaviorProfile'

interface BehaviorProfileEditorProps {
  value?: BehaviorProfile
  onChange?: (profile: BehaviorProfile) => void
}

const useStyles = createStyles(({ css }) => ({
  shell: css`
    border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: 8px;
    background: color-mix(in srgb, var(--background) 96%, var(--foreground) 4%);
    overflow: hidden;
  `,
  intro: css`
    display: flex;
    justify-content: space-between;
    gap: 20px;
    padding: 14px 16px;
    border-bottom: 1px solid
      color-mix(in srgb, var(--foreground) 7%, transparent);
  `,
  introCopy: css`
    min-width: 0;
  `,
  introTitle: css`
    display: block;
    margin-bottom: 3px;
    color: var(--text);
    font-size: 13px;
    font-weight: 600;
  `,
  introDesc: css`
    display: block;
    max-width: 450px;
    color: var(--text-muted);
    font-size: 11px;
    line-height: 1.55;
  `,
  version: css`
    flex: none;
    align-self: flex-start;
    padding: 3px 7px;
    border: 1px solid color-mix(in srgb, var(--primary) 25%, transparent);
    border-radius: 999px;
    color: var(--primary);
    background: color-mix(in srgb, var(--primary) 7%, transparent);
    font-size: 10px;
    font-weight: 600;
  `,
  presets: css`
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 7px;
    padding: 12px 16px 4px;

    @media (max-width: 640px) {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  `,
  preset: css`
    min-width: 0;
    padding: 8px 9px;
    border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: 6px;
    color: var(--text-secondary);
    background: var(--background);
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.15s,
      background 0.15s,
      color 0.15s;

    &:hover {
      border-color: color-mix(in srgb, var(--primary) 40%, transparent);
      color: var(--text);
    }
  `,
  presetActive: css`
    border-color: color-mix(in srgb, var(--primary) 55%, transparent);
    color: var(--primary);
    background: color-mix(in srgb, var(--primary) 7%, var(--background));
  `,
  presetName: css`
    display: block;
    margin-bottom: 2px;
    font-size: 11px;
    font-weight: 600;
  `,
  presetDesc: css`
    display: block;
    overflow: hidden;
    color: var(--text-muted);
    font-size: 9px;
    line-height: 1.35;
  `,
  traitGrid: css`
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 12px 16px 16px;

    @media (max-width: 640px) {
      grid-template-columns: 1fr;
    }
  `,
  trait: css`
    min-width: 0;
    padding: 10px 12px 8px;
    border: 1px solid color-mix(in srgb, var(--foreground) 7%, transparent);
    border-radius: 6px;
    background: var(--background);
  `,
  traitHead: css`
    display: flex;
    justify-content: space-between;
    gap: 8px;
    align-items: baseline;
  `,
  traitLabel: css`
    color: var(--text);
    font-size: 12px;
    font-weight: 600;
  `,
  score: css`
    color: var(--primary);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  `,
  traitDesc: css`
    display: block;
    margin-top: 1px;
    color: var(--text-muted);
    font-size: 9px;
  `,
  slider: css`
    margin: 11px 3px 2px;
  `,
  anchors: css`
    display: flex;
    justify-content: space-between;
    color: var(--text-tertiary);
    font-size: 9px;
  `,
}))

export default function BehaviorProfileEditor({
  value,
  onChange,
}: BehaviorProfileEditorProps) {
  const { styles } = useStyles()
  const profile = cloneBehaviorProfile(value)

  const setScore = (
    key: keyof Omit<BehaviorProfile, 'version'>,
    score: number,
  ) => {
    onChange?.({ ...profile, [key]: score })
  }

  return (
    <div className={styles.shell}>
      <div className={styles.intro}>
        <div className={styles.introCopy}>
          <span className={styles.introTitle}>人格光谱</span>
          <span className={styles.introDesc}>
            这组结构化倾向会在部署时转成稳定的系统上下文；它影响判断，但不会赋予越级、通信或数据权限。
          </span>
        </div>
        <span className={styles.version}>Schema v{profile.version}</span>
      </div>

      <div className={styles.presets} aria-label="行为人格预设">
        {BEHAVIOR_PRESETS.map((preset) => {
          const active = profilesEqual(profile, preset.profile)
          return (
            <button
              key={preset.id}
              type="button"
              className={`${styles.preset} ${active ? styles.presetActive : ''}`}
              aria-pressed={active}
              title={preset.description}
              onClick={() => {
                onChange?.(cloneBehaviorProfile(preset.profile))
              }}
            >
              <span className={styles.presetName}>{preset.name}</span>
              <span className={styles.presetDesc}>{preset.description}</span>
            </button>
          )
        })}
      </div>

      <div className={styles.traitGrid}>
        {BEHAVIOR_TRAITS.map((trait) => (
          <div className={styles.trait} key={trait.key}>
            <div className={styles.traitHead}>
              <span className={styles.traitLabel}>{trait.label}</span>
              <span className={styles.score}>{profile[trait.key]}</span>
            </div>
            <span className={styles.traitDesc}>{trait.description}</span>
            <Slider
              className={styles.slider}
              min={0}
              max={100}
              value={profile[trait.key]}
              tooltip={{ formatter: (next) => `${next ?? 0} / 100` }}
              aria-label={trait.label}
              onChange={(score) => {
                setScore(trait.key, score)
              }}
            />
            <div className={styles.anchors}>
              <span>{trait.low}</span>
              <span>{trait.high}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
