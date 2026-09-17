import { memo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createStyles } from 'antd-style'
import {
  FolderIcon,
  FolderOpenIcon,
  FileTextIcon,
  FileArrowDownIcon,
  LinkIcon,
} from '@phosphor-icons/react'
import { useDirEntries } from '@/queries/useAgentFiles'
import type { FileEntry } from '@/api/agent-files'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  list: css`
    list-style: none;
    padding: 0;
    margin: 0;
    overflow-y: auto;
    flex: 1 1 auto;
    font-family: ${tk.fontMono};
    font-size: 12.5px;
  `,
  node: css`
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 8px;
    cursor: pointer;
    user-select: none;
    color: ${tk.textSecondary};
    border-radius: 2px;
    &:hover {
      background: ${tk.inkLight};
    }
    &.selected {
      background: ${tk.inkLight};
      color: ${tk.text};
    }
  `,
  icon: css`
    flex-shrink: 0;
    display: inline-flex;
    color: ${tk.textTertiary};
  `,
  name: css`
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  symlinkTarget: css`
    color: ${tk.textMuted};
    font-size: 11px;
    margin-left: 4px;
  `,
  placeholder: css`
    padding: 4px 16px;
    color: ${tk.textMuted};
    font-style: italic;
  `,
}))

interface TreeProps {
  agentName: string
  selectedPath: string | null
  onSelect: (path: string, entry: FileEntry) => void
}

export default function CwdFileTree(props: TreeProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  // Root directory load — empty path = cwd root.
  const root = useDirEntries(props.agentName, '', true)

  if (root.isLoading) {
    return <div className={styles.placeholder}>{t('agentChat.loading')}</div>
  }
  if (root.isError || !root.data) {
    // Bubble up to panel: returning null here hides only the tree, panel
    // wrapper will hide entirely when error is unrecoverable.
    return null
  }

  return (
    <ul className={styles.list} role="tree" aria-label={t('agentChat.workspace')}>
      {root.data.entries.map((entry) => (
        <CwdFileNode
          key={entry.name}
          entry={entry}
          depth={0}
          basePath=""
          agentName={props.agentName}
          selectedPath={props.selectedPath}
          onSelect={props.onSelect}
        />
      ))}
    </ul>
  )
}

interface NodeProps {
  entry: FileEntry
  depth: number
  basePath: string    // parent directory path, '' for root
  agentName: string
  selectedPath: string | null
  onSelect: (path: string, entry: FileEntry) => void
}

// memo: entries are stable references from react-query cache; re-renders
// only fire when entry/selected/expanded props change.
const CwdFileNode = memo(function CwdFileNode(props: NodeProps) {
  const { styles } = useStyles()
  const { t } = useTranslation()
  const { entry, depth, basePath, agentName, selectedPath, onSelect } = props
  const [expanded, setExpanded] = useState(false)

  // dirPath for this node: relative to cwd root.
  // Root entries have basePath === '' so their path is just entry.name;
  // nested nodes prepend parent path.
  const fullPath = basePath ? `${basePath}/${entry.name}` : entry.name

  const isDir = entry.type === 'directory'
  const children = useDirEntries(agentName, fullPath, isDir && expanded)

  const handleClick = () => {
    if (isDir) {
      setExpanded((v) => !v)
    } else {
      onSelect(fullPath, entry)
    }
  }

  const isSelected = selectedPath === fullPath

  const icon = renderIcon(entry, expanded)

  return (
    <li role="treeitem" aria-expanded={isDir ? expanded : undefined}>
      <div
        className={`${styles.node} ${isSelected ? 'selected' : ''}`}
        style={{ paddingLeft: 8 + depth * 12 }}
        onClick={handleClick}
      >
        <span className={styles.icon}>{icon}</span>
        <span className={styles.name}>{entry.name}</span>
        {entry.type === 'symlink' && entry.target && (
          <span className={styles.symlinkTarget}>→ {entry.target}</span>
        )}
      </div>
      {isDir && expanded && (
        <ul
          className={styles.list}
          role="group"
          style={{ paddingLeft: 0, margin: 0, listStyle: 'none' }}
        >
          {children.isLoading && (
            <li className={styles.placeholder} style={{ paddingLeft: 16 + depth * 12 }}>
              {t('agentChat.loading')}
            </li>
          )}
          {children.isError && (
            <li className={styles.placeholder} style={{ paddingLeft: 16 + depth * 12 }}>
              {t('agentChat.loadFailPrefix')}<span onClick={() => children.refetch()}>{t('agentChat.retryLink')}</span>
            </li>
          )}
          {children.data?.entries.map((child) => (
            <CwdFileNode
              key={child.name}
              entry={child}
              depth={depth + 1}
              basePath={fullPath}
              agentName={agentName}
              selectedPath={selectedPath}
              onSelect={onSelect}
            />
          ))}
        </ul>
      )}
    </li>
  )
})

function renderIcon(entry: FileEntry, expanded: boolean) {
  switch (entry.type) {
    case 'directory':
      return expanded ? <FolderOpenIcon size={14} /> : <FolderIcon size={14} />
    case 'symlink':
      return <LinkIcon size={14} />
    case 'file':
      // Heuristic: common download-likely extensions (zip, gz, pdf) get the
      // download icon, everything else gets the document icon.
      if (/\.(zip|gz|tar|pdf|7z|rar)$/i.test(entry.name)) {
        return <FileArrowDownIcon size={14} />
      }
      return <FileTextIcon size={14} />
    default:
      return <FileTextIcon size={14} />
  }
}
