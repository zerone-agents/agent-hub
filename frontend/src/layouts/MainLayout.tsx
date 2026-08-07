import { Outlet } from 'react-router-dom'
import { createStyles } from 'antd-style'
import AppHeader from '@/components/AppHeader'
import AppSidebar from '@/components/AppSidebar'
import { useSidebarCollapsed } from '@/hooks/useSidebarCollapsed'

const useStyles = createStyles(({ css }) => ({
  layout: css`
    height: 100vh;
    display: flex;
    overflow: hidden;
    background: var(--background);
  `,
  right: css`
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  `,
  main: css`
    flex: 1;
    width: 100%;
    margin: 0 auto;
    padding: 24px 32px;
    overflow-y: auto;
    background: var(--background);
    @media (max-width: 768px) {
      padding: 16px 16px;
    }
  `
}))

export default function MainLayout() {
  const { styles } = useStyles()
  const { collapsed, toggle } = useSidebarCollapsed()
  return (
    <div className={styles.layout}>
      <AppSidebar collapsed={collapsed} />
      <div className={styles.right}>
        <AppHeader onToggleSidebar={toggle} />
        <main className={styles.main}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
