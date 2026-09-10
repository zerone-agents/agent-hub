import { useMemo, useState } from 'react'
import { Select, Slider } from 'antd'
import { CaretDownIcon, SlidersHorizontalIcon } from '@phosphor-icons/react'
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
  template: css`
    padding: 14px 16px;
  `,
  templateLabelRow: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 7px;
  `,
  templateLabel: css`
    color: var(--text-secondary);
    font-size: 11px;
    font-weight: 600;
  `,
  templateStatus: css`
    color: var(--text-tertiary);
    font-size: 10px;
  `,
  templateSelect: css`
    width: 100%;

    .ant-select-selector {
      min-height: 42px !important;
      padding-inline: 12px !important;
      border-color: color-mix(in srgb, var(--foreground) 11%, transparent) !important;
      background: var(--background) !important;
      box-shadow: none !important;
    }

    &.ant-select-focused .ant-select-selector,
    &:hover .ant-select-selector {
      border-color: color-mix(in srgb, var(--primary) 55%, transparent) !important;
    }
  `,
  option: css`
    display: grid;
    gap: 2px;
    padding-block: 3px;
  `,
  optionName: css`
    color: var(--text);
    font-size: 12px;
    font-weight: 600;
  `,
  optionDesc: css`
    color: var(--text-muted);
    font-size: 10px;
    line-height: 1.4;
  `,
  templateFoot: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-top: 8px;
  `,
  templateDescription: css`
    min-width: 0;
    overflow: hidden;
    color: var(--text-muted);
    font-size: 10px;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  tuneButton: css`
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 5px;
    padding: 0;
    border: 0;
    color: var(--primary);
    background: transparent;
    font-size: 10px;
    font-weight: 600;
    cursor: pointer;

    &:focus-visible {
      outline: 2px solid color-mix(in srgb, var(--primary) 45%, transparent);
      outline-offset: 3px;
      border-radius: 3px;
    }
  `,
  traitGrid: css`
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 2px 16px 16px;

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
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const profile = cloneBehaviorProfile(value)
  const selectedPreset = BEHAVIOR_PRESETS.find((preset) =>
    profilesEqual(profile, preset.profile),
  )
  const selectedTemplateId = selectedPreset?.id ?? 'custom'
  const templateOptions = useMemo(
    () => [
      ...BEHAVIOR_PRESETS.map((preset) => ({
        value: preset.id,
        label: preset.name,
        description: preset.description,
      })),
      {
        value: 'custom',
        label: '自定义参数',
        description: '点击“调整参数”后手动修改，自动生成',
        disabled: true,
      },
    ],
    [],
  )

  const setScore = (
    key: keyof Omit<BehaviorProfile, 'version'>,
    score: number,
  ) => {
    onChange?.({ ...profile, [key]: score })
  }

  const selectTemplate = (templateId: string) => {
    if (templateId === 'custom') {
      setAdvancedOpen(true)
      return
    }
    const preset = BEHAVIOR_PRESETS.find((item) => item.id === templateId)
    if (preset) {
      onChange?.(cloneBehaviorProfile(preset.profile))
    }
  }

  return (
    <div className={styles.shell}>
      <div className={styles.intro}>
        <div className={styles.introCopy}>
          <span className={styles.introTitle}>结构化投影（兼容）</span>
          <span className={styles.introDesc}>
            用于旧版行为逻辑、筛选和观察。人格原稿才是主要来源；这些数值不会赋予越级、通信或数据权限。
          </span>
        </div>
        <span className={styles.version}>Schema v{profile.version}</span>
      </div>

      <div className={styles.template}>
        <div className={styles.templateLabelRow}>
          <label className={styles.templateLabel} htmlFor="behavior-profile-template">
            投影预设
          </label>
          <span className={styles.templateStatus}>
            {selectedPreset ? '已套用模板' : '已调整为自定义参数'}
          </span>
        </div>
        <Select
          id="behavior-profile-template"
          aria-label="投影预设"
          className={styles.templateSelect}
          value={selectedTemplateId}
          options={templateOptions}
          suffix={<CaretDownIcon size={14} />}
          showSearch={{ optionFilterProp: 'label' }}
          popupMatchSelectWidth
          optionRender={(option) => (
            <div className={styles.option}>
              <span className={styles.optionName}>{option.label}</span>
              <span className={styles.optionDesc}>
                {option.data.description}
              </span>
            </div>
          )}
          onChange={selectTemplate}
        />
        <div className={styles.templateFoot}>
          <span className={styles.templateDescription}>
            {selectedPreset?.description ?? '当前参数是该 Agent 的独立自定义配置'}
          </span>
          <button
            type="button"
            className={styles.tuneButton}
            aria-expanded={advancedOpen}
            onClick={() => { setAdvancedOpen((open) => !open) }}
          >
            <SlidersHorizontalIcon size={13} />
            {advancedOpen ? '收起参数' : '调整参数'}
          </button>
        </div>
      </div>

      {advancedOpen && <div className={styles.traitGrid} aria-label="高级人格参数">
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
      </div>}
    </div>
  )
}
