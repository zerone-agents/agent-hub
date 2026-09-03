import { GithubLogoIcon, GlobeIcon } from '@phosphor-icons/react'
import { Tooltip } from 'antd'
import { createStyles } from 'antd-style'

const GITHUB_REPO_URL = 'https://github.com/zerone-agents/agent-hub'
const OFFICIAL_SITE_URL = 'https://www.zerone.run/'

// 与 ThemeControls 的幽灵按钮保持一致的视觉样式
const useStyles = createStyles(({ css }) => ({
  links: css`
    display: flex;
    align-items: center;
    gap: 4px;
  `,
  link: css`
    width: 34px;
    height: 34px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    background: transparent;
    cursor: pointer;
    transition:
      color 160ms ease,
      background 160ms ease,
      border-color 160ms ease,
      transform 160ms ease;

    &:hover {
      color: var(--primary);
      background: var(--accent);
      border-color: var(--border);
    }

    &:active {
      transform: translateY(1px);
    }

    &:focus-visible {
      outline: 2px solid var(--ring);
      outline-offset: 2px;
    }
  `
}))

const LINKS = [
  { href: GITHUB_REPO_URL, label: 'GitHub 仓库', Icon: GithubLogoIcon },
  { href: OFFICIAL_SITE_URL, label: '官方网站', Icon: GlobeIcon }
]

export default function HeaderLinks() {
  const { styles } = useStyles()

  return (
    <div className={styles.links} aria-label="相关链接">
      {LINKS.map(({ href, label, Icon }) => (
        <Tooltip key={href} title={label}>
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className={styles.link}
            aria-label={label}
          >
            <Icon size={18} />
          </a>
        </Tooltip>
      ))}
    </div>
  )
}
