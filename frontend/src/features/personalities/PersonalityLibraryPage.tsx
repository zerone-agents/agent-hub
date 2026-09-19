import { useMemo, useState } from 'react'
import { Empty, Popconfirm, Spin, Tag } from 'antd'
import {
  ClockCounterClockwiseIcon,
  FingerprintIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
  UsersIcon,
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import NameSearch from '@/components/NameSearch'
import PrimaryButton from '@/components/PrimaryButton'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { Personality } from '@/api/personalities'
import {
  useDeletePersonality,
  usePersonalities,
  usePersonality,
} from '@/queries/usePersonalities'
import { formatTime } from '@/utils/time'
import PersonalityForm from './PersonalityForm'

const useStyles = createStyles(({ css }) => ({
  page: css`
    min-height: calc(100vh - 100px);
    animation: personalityIn 0.28s ease;
    @keyframes personalityIn {
      from {
        opacity: 0;
        transform: translateY(5px);
      }
    }
    @media (prefers-reduced-motion: reduce) {
      animation: none;
    }
  `,
  pageHead: css`
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 20px;
    margin-bottom: 22px;
    @media (max-width: 720px) {
      flex-direction: column;
    }
  `,
  pageTitle: css`
    font-size: 2.25rem;
    font-weight: 720;
    color: var(--text);
    letter-spacing: -0.035em;
    line-height: 1.1;
  `,
  pageSub: css`
    margin-top: 6px;
    color: var(--text-tertiary);
    font-size: 0.9375rem;
  `,
  frame: css`
    display: grid;
    grid-template-columns: minmax(270px, 330px) minmax(0, 1fr);
    min-height: 620px;
    border: 1px solid var(--border);
    border-radius: 14px;
    background: var(--card);
    overflow: hidden;
    box-shadow: var(--elevation-1);
    @media (max-width: 880px) {
      grid-template-columns: 1fr;
    }
  `,
  index: css`
    min-width: 0;
    border-right: 1px solid var(--border);
    background: var(--background);
    @media (max-width: 880px) {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
  `,
  indexHead: css`
    padding: 16px;
    border-bottom: 1px solid var(--border);
  `,
  indexLabel: css`
    display: flex;
    justify-content: space-between;
    margin-bottom: 11px;
    color: var(--text-muted);
    font:
      600 11px/1 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  `,
  list: css`
    max-height: 680px;
    overflow-y: auto;
  `,
  row: css`
    width: 100%;
    display: grid;
    grid-template-columns: 30px minmax(0, 1fr) auto;
    gap: 10px;
    align-items: start;
    padding: 14px 16px;
    border: 0;
    border-bottom: 1px solid
      color-mix(in srgb, var(--foreground) 6%, transparent);
    background: transparent;
    text-align: left;
    cursor: pointer;
    transition: background 0.16s ease;
    &:hover {
      background: var(--ink-subtle);
    }
    &:focus-visible {
      outline: 2px solid var(--primary);
      outline-offset: -3px;
    }
  `,
  rowActive: css`
    background: color-mix(in srgb, var(--primary) 8%, var(--background));
    box-shadow: inset 3px 0 0 var(--primary);
  `,
  rowMuted: css`
    opacity: 0.62;
  `,
  rowTitle: css`
    display: block;
    color: var(--text);
    font-size: 14px;
    font-weight: 650;
    line-height: 1.3;
  `,
  rowName: css`
    display: block;
    margin-top: 3px;
    overflow: hidden;
    color: var(--text-muted);
    font:
      11px/1.35 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
    text-overflow: ellipsis;
  `,
  rowMeta: css`
    display: grid;
    justify-items: end;
    gap: 5px;
    color: var(--text-muted);
    font-size: 10px;
  `,
  inspector: css`
    min-width: 0;
    display: flex;
    flex-direction: column;
  `,
  inspectorHead: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 18px;
    padding: 22px 24px 18px;
    border-bottom: 1px solid var(--border);
  `,
  inspectorTitle: css`
    margin: 0;
    color: var(--text);
    font-size: 23px;
    font-weight: 680;
    letter-spacing: -0.025em;
  `,
  description: css`
    max-width: 680px;
    margin: 5px 0 0;
    color: var(--text-tertiary);
    font-size: 13px;
    line-height: 1.55;
  `,
  actions: css`
    display: flex;
    flex: none;
    gap: 6px;
  `,
  iconButton: css`
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border: 1px solid var(--border);
    border-radius: 7px;
    color: var(--text-secondary);
    background: var(--background);
    cursor: pointer;
    &:hover {
      color: var(--primary);
      border-color: color-mix(in srgb, var(--primary) 38%, var(--border));
    }
    &:focus-visible {
      outline: 2px solid var(--primary);
      outline-offset: 2px;
    }
  `,
  danger: css`
    &:hover {
      color: var(--destructive);
      border-color: color-mix(in srgb, var(--destructive) 40%, var(--border));
    }
  `,
  metaStrip: css`
    display: flex;
    flex-wrap: wrap;
    gap: 18px;
    padding: 11px 24px;
    border-bottom: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 11px;
  `,
  metaItem: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
  `,
  content: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) 190px;
    min-height: 480px;
    @media (max-width: 1100px) {
      grid-template-columns: 1fr;
    }
  `,
  manuscriptWrap: css`
    padding: 24px;
    min-width: 0;
  `,
  manuscript: css`
    position: relative;
    min-height: 420px;
    padding: 30px 32px 34px 48px;
    border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: 4px 12px 12px 4px;
    background: color-mix(in srgb, var(--background) 97%, var(--primary) 3%);
    box-shadow: 0 12px 28px -24px
      color-mix(in srgb, var(--foreground) 45%, transparent);
    &::before {
      content: '';
      position: absolute;
      left: 24px;
      top: 0;
      bottom: 0;
      width: 1px;
      background: color-mix(in srgb, var(--primary) 38%, transparent);
    }
  `,
  folio: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 22px;
    color: var(--primary);
    font:
      650 10px/1 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  `,
  prompt: css`
    margin: 0;
    color: var(--text);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font:
      15px/1.86 'Songti SC',
      'Noto Serif CJK SC',
      Georgia,
      serif;
  `,
  history: css`
    padding: 24px 18px;
    border-left: 1px solid var(--border);
    background: var(--background);
    @media (max-width: 1100px) {
      border-left: 0;
      border-top: 1px solid var(--border);
    }
  `,
  historyTitle: css`
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 18px;
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 650;
  `,
  version: css`
    position: relative;
    padding: 0 0 19px 18px;
    color: var(--text-muted);
    font-size: 11px;
    &::before {
      content: '';
      position: absolute;
      left: 3px;
      top: 5px;
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--primary);
    }
    &::after {
      content: '';
      position: absolute;
      left: 5.5px;
      top: 12px;
      bottom: 0;
      width: 1px;
      background: var(--border);
    }
    &:last-child::after {
      display: none;
    }
  `,
  versionName: css`
    display: block;
    margin-bottom: 4px;
    color: var(--text-secondary);
    font:
      650 11px/1 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
  `,
  empty: css`
    min-height: 560px;
    display: grid;
    place-items: center;
  `,
  loading: css`
    min-height: 560px;
    display: grid;
    place-items: center;
  `,
}))

export default function PersonalityLibraryPage() {
  const { styles, cx } = useStyles()
  const canWrite = useCanWrite()
  const { data: personalities = [], isLoading } = usePersonalities()
  const deletePersonality = useDeletePersonality()
  const [selectedName, setSelectedName] = useState('')
  const [keywords, setKeywords] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Personality | null>(null)
  const effectiveSelectedName = personalities.some(
    (item) => item.name === selectedName,
  )
    ? selectedName
    : (personalities[0]?.name ?? '')
  const { data: selectedDetail } = usePersonality(effectiveSelectedName)

  const filtered = useMemo(() => {
    const query = keywords.trim().toLowerCase()
    if (!query) return personalities
    return personalities.filter((item) =>
      [item.name, item.title, item.description, item.prompt].some((value) =>
        value.toLowerCase().includes(query),
      ),
    )
  }, [keywords, personalities])

  const selected =
    selectedDetail ??
    personalities.find((item) => item.name === effectiveSelectedName)

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>人格库</div>
          <div className={styles.pageSub}>
            用完整提示词塑造人物；版本与 Agent 快照让角色演化可控、可复盘。
          </div>
        </div>
        {canWrite && (
          <PrimaryButton
            icon={<PlusIcon size={16} weight="bold" />}
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            新建人格
          </PrimaryButton>
        )}
      </div>

      <div className={styles.frame}>
        <aside className={styles.index}>
          <div className={styles.indexHead}>
            <div className={styles.indexLabel}>
              <span>Personality index</span>
              <span>{personalities.length}</span>
            </div>
            <NameSearch
              placeholder="搜索人格或原稿"
              realtime
              onSearch={setKeywords}
            />
          </div>
          {isLoading ? (
            <div className={styles.loading}>
              <Spin />
            </div>
          ) : (
            <div className={styles.list}>
              {filtered.map((item) => (
                <button
                  key={item.name}
                  type="button"
                  className={cx(
                    styles.row,
                    item.name === effectiveSelectedName && styles.rowActive,
                    !item.enabled && styles.rowMuted,
                  )}
                  onClick={() => {
                    setSelectedName(item.name)
                  }}
                >
                  <FingerprintIcon size={18} aria-hidden="true" />
                  <span>
                    <span className={styles.rowTitle}>{item.title}</span>
                    <span className={styles.rowName}>{item.name}</span>
                  </span>
                  <span className={styles.rowMeta}>
                    <span>v{item.currentVersion}</span>
                    <span>{item.usageCount} 人</span>
                  </span>
                </button>
              ))}
              {!filtered.length && (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="没有匹配的人格"
                />
              )}
            </div>
          )}
        </aside>

        <section className={styles.inspector}>
          {!selected ? (
            <div className={styles.empty}>
              <Empty description="选择一个人格查看原稿" />
            </div>
          ) : (
            <>
              <div className={styles.inspectorHead}>
                <div>
                  <h2 className={styles.inspectorTitle}>{selected.title}</h2>
                  <p className={styles.description}>
                    {selected.description || '尚未填写人格说明。'}
                  </p>
                </div>
                {canWrite && (
                  <div className={styles.actions}>
                    <button
                      type="button"
                      className={styles.iconButton}
                      title="编辑人格"
                      onClick={() => {
                        setEditing(selected)
                        setFormOpen(true)
                      }}
                    >
                      <PencilSimpleIcon size={16} />
                    </button>
                    {!selected.isBuiltin && (
                      <Popconfirm
                        title="确认删除人格？"
                        description="已选用此人格的 Agent 会保留自己的提示词快照。"
                        okText="删除"
                        okButtonProps={{ danger: true }}
                        cancelText="取消"
                        onConfirm={() => {
                          deletePersonality.mutate(selected.name)
                          setSelectedName('')
                        }}
                      >
                        <button
                          type="button"
                          className={cx(styles.iconButton, styles.danger)}
                          title="删除人格"
                        >
                          <TrashIcon size={16} />
                        </button>
                      </Popconfirm>
                    )}
                  </div>
                )}
              </div>
              <div className={styles.metaStrip}>
                <span className={styles.metaItem}>
                  <FingerprintIcon size={14} />
                  {selected.name}
                </span>
                <span className={styles.metaItem}>
                  <UsersIcon size={14} />
                  {selected.usageCount} 个 Agent 使用
                </span>
                <Tag
                  variant="filled"
                  color={selected.enabled ? 'success' : 'default'}
                >
                  {selected.enabled ? '可选用' : '已停用'}
                </Tag>
                {selected.isBuiltin && <Tag variant="filled">系统人格</Tag>}
              </div>
              <div className={styles.content}>
                <div className={styles.manuscriptWrap}>
                  <article className={styles.manuscript}>
                    <div className={styles.folio}>
                      <span>Personality manuscript</span>
                      <span>v{selected.currentVersion}</span>
                    </div>
                    <pre className={styles.prompt}>{selected.prompt}</pre>
                  </article>
                </div>
                <aside className={styles.history}>
                  <div className={styles.historyTitle}>
                    <ClockCounterClockwiseIcon size={15} />
                    版本记录
                  </div>
                  {(selected.versions ?? []).map((version) => (
                    <div key={version.id} className={styles.version}>
                      <span className={styles.versionName}>
                        VERSION {version.version}
                      </span>
                      <span>{version.changeNote || '更新人格原稿'}</span>
                      <br />
                      <span>{formatTime(version.createdAt)}</span>
                    </div>
                  ))}
                  {!selected.versions?.length && (
                    <span className={styles.description}>
                      正在读取版本记录…
                    </span>
                  )}
                </aside>
              </div>
            </>
          )}
        </section>
      </div>
      <PersonalityForm
        open={formOpen}
        editing={editing}
        onClose={() => {
          setFormOpen(false)
        }}
      />
    </div>
  )
}
