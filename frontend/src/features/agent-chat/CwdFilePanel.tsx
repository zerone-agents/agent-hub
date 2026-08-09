import { useEffect, useState } from 'react'
import { createStyles } from 'antd-style'
import { FolderSimpleIcon, SidebarSimpleIcon } from '@phosphor-icons/react'
import { useDirEntries } from '@/queries/useAgentFiles'
import type { FileEntry } from '@/api/agent-files'
import { tokens as t } from '@/styles/tokens'
import CwdFileTree from './CwdFileTree'
import CwdFilePreview from './CwdFilePreview'

const useStyles = createStyles(({ css }) => ({
  // Expanded: 280px wide column with header + tree + preview
  expanded: css`
    flex: 0 0 280px;
    display: flex;
    flex-direction: column;
    border-left: 1px solid ${t.inkLighter};
    background: ${t.surface};
    overflow: hidden;
    @media (max-width: 768px) {
      display: none;
    }
  `,
  // Collapsed: 36px rail with single centered icon
  collapsed: css`
    flex: 0 0 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-left: 1px solid ${t.inkLighter};
    background: ${t.surface};
    cursor: pointer;
    color: ${t.textTertiary};
    &:hover {
      background: ${t.surfaceHover};
      color: ${t.text};
    }
    @media (max-width: 768px) {
      display: none;
    }
  `,
  header: css`
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 12px;
    border-bottom: 1px solid ${t.inkLighter};
    font-size: 12px;
    font-weight: 600;
    color: ${t.text};
  `,
  headerTitle: css`
    flex: 1;
  `,
  collapseBtn: css`
    display: inline-flex;
    align-items: center;
    color: ${t.textTertiary};
    cursor: pointer;
    border: none;
    background: transparent;
    padding: 2px;
    border-radius: 2px;
    &:hover {
      background: ${t.inkLight};
      color: ${t.text};
    }
  `,
  treeWrap: css`
    flex: 2 1 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  `,
}))

interface Props {
  agentName: string
}

function storageKey(agentName: string) {
  return `agent-chat.cwd-panel.${agentName}.expanded`
}

function readExpanded(agentName: string): boolean {
  const v = localStorage.getItem(storageKey(agentName))
  // Default false: first visit shows collapsed rail, avoiding chat squeeze.
  return v === 'true'
}

function writeExpanded(agentName: string, expanded: boolean) {
  localStorage.setItem(storageKey(agentName), expanded ? 'true' : 'false')
}

export default function CwdFilePanel({ agentName }: Props) {
  const { styles } = useStyles()
  const [expanded, setExpanded] = useState<boolean>(() => readExpanded(agentName))
  const [selected, setSelected] = useState<{ path: string; entry: FileEntry } | null>(null)

  // When agent changes, drop the stale selection. The expanded state is
  // per-agent (separate localStorage key) so we re-read on agentName change.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- sync reset on agent switch; both setStates are coupled to agentName change
    setSelected(null)
    setExpanded(readExpanded(agentName))
  }, [agentName])

  // Probe root load: if it errors we hide the entire panel (decorator
  // pattern, mirrors AgentDetailBar). enabled only when expanded so the
  // collapsed rail never fires requests.
  const root = useDirEntries(agentName, '', expanded)

  if (expanded && (root.isError || (!root.isLoading && !root.data))) {
    // Hide silently — agent likely undeployed or runtime unreachable.
    return null
  }

  if (!expanded) {
    return (
      <div
        className={styles.collapsed}
        role="button"
        aria-label="展开 Agent 工作区"
        title="展开 Agent 工作区"
        onClick={() => {
          const next = true
          writeExpanded(agentName, next)
          setExpanded(next)
        }}
      >
        <FolderSimpleIcon size={18} />
      </div>
    )
  }

  const handleToggle = () => {
    const next = false
    writeExpanded(agentName, next)
    setExpanded(next)
  }

  return (
    <div className={styles.expanded}>
      <div className={styles.header}>
        <FolderSimpleIcon size={14} />
        <span className={styles.headerTitle}>Agent 工作区</span>
        <button
          type="button"
          className={styles.collapseBtn}
          aria-label="折叠 Agent 工作区"
          title="折叠"
          onClick={handleToggle}
        >
          <SidebarSimpleIcon size={14} />
        </button>
      </div>
      <div className={styles.treeWrap}>
        <CwdFileTree
          agentName={agentName}
          selectedPath={selected?.path ?? null}
          onSelect={(path, entry) => { setSelected({ path, entry }); }}
        />
      </div>
      <CwdFilePreview
        agentName={agentName}
        // CwdFilePreview expects file.name to be the cwd-relative path; the
        // tree already passes that as `path`. Mutate entry.name before
        // forwarding so preview can build the content URL.
        selectedFile={selected ? { ...selected.entry, name: selected.path } : null}
      />
    </div>
  )
}
