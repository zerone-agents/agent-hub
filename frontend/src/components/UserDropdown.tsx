import { useState } from 'react'
import { Avatar, Dropdown, type MenuProps } from 'antd'
import { LockIcon, SignOutIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { useAuthStore } from '@/stores/auth'
import { tokens as t } from '@/styles/tokens'
import ChangePasswordModal from '@/features/users/ChangePasswordModal'

// 用户胶囊 + 下拉菜单（与 AppHeader/聊天页共用的页眉用户区）：内置「修改密码」
// （含 ChangePasswordModal）与「退出登录」，extraItems 注入在修改密码之前
// （如管理页的用户管理 / CLI Tokens / AIGC 标识配置）。
const useStyles = createStyles(({ css }) => ({
  userArea: css`
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 12px 4px 16px;
    border-radius: 20px;
    cursor: pointer;
    transition: background 0.15s;
    &:hover {
      background: ${t.surfaceHover};
    }
  `,
  userName: css`
    font-size: ${t.textSm};
    font-weight: 500;
    color: ${t.textSecondary};
    @media (max-width: 768px) {
      display: none;
    }
  `,
  avatar: css`
    background: var(--primary);
    color: var(--primary-foreground);
  `
}))

function getAvatarInitial(name?: string) {
  const firstCharacter = Array.from(name?.trim() ?? '')[0]
  return firstCharacter ? firstCharacter.toLocaleUpperCase() : 'U'
}

interface UserDropdownProps {
  /** 追加在「修改密码」之前的菜单项（管理页专属入口）。 */
  extraItems?: MenuProps['items']
}

export default function UserDropdown({ extraItems }: UserDropdownProps) {
  const { styles } = useStyles()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const [pwdModalOpen, setPwdModalOpen] = useState(false)

  const handleLogout = async () => {
    await logout()
    await navigate('/login')
  }

  const items: MenuProps['items'] = [
    ...(extraItems ?? []),
    {
      key: 'change-password',
      icon: <LockIcon size={14} />,
      label: t('components.userDropdown.changePassword'),
      onClick: () => { setPwdModalOpen(true); }
    },
    {
      key: 'logout',
      icon: <SignOutIcon size={14} />,
      label: t('common.logout'),
      onClick: () => { void handleLogout(); }
    }
  ]

  return (
    <>
      <Dropdown menu={{ items }} trigger={['click']}>
        <div className={styles.userArea}>
          <span className={styles.userName}>{user?.name ?? 'Admin'}</span>
          <Avatar className={styles.avatar} size={28}>
            {getAvatarInitial(user?.name)}
          </Avatar>
        </div>
      </Dropdown>
      <ChangePasswordModal open={pwdModalOpen} onClose={() => { setPwdModalOpen(false); }} />
    </>
  )
}
