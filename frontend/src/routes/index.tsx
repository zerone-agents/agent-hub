import { createBrowserRouter, Navigate } from 'react-router'
import RequireAuth from './RequireAuth'
import MainLayout from '@/layouts/MainLayout'
import NotFound from '@/components/NotFound'
import AgentChatPage from '@/features/agent-chat/AgentChatPage'
import ChatHomePage from '@/features/agent-chat/ChatHomePage'

export const router = createBrowserRouter(
  [
    {
      path: '/login',
      lazy: () => import('@/features/login/LoginPage').then((m) => ({ Component: m.default }))
    },
    {
      path: '/setup',
      lazy: () => import('@/features/setup/SetupPage').then((m) => ({ Component: m.default }))
    },
    {
      path: '/register',
      lazy: () => import('@/features/register/RegisterPage').then((m) => ({ Component: m.default }))
    },
    {
      path: '/agents/chat',
      element: (
        <RequireAuth allowGuest>
          <ChatHomePage />
        </RequireAuth>
      )
    },
    {
      path: '/agents/:name/chat',
      element: (
        <RequireAuth allowGuest>
          <AgentChatPage />
        </RequireAuth>
      )
    },
    {
      path: '/',
      element: (
        <RequireAuth>
          <MainLayout />
        </RequireAuth>
      ),
      children: [
        { index: true, element: <Navigate to="/dashboard" replace /> },
        {
          path: 'dashboard',
          lazy: () =>
            import('@/features/dashboard/DashboardPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'agents',
          lazy: () =>
            import('@/features/agents/AgentListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'runs/:runId?',
          lazy: () =>
            import('@/features/runs/RunCenterPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'personalities',
          lazy: () =>
            import('@/features/personalities/PersonalityLibraryPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'relations',
          lazy: () =>
            import('@/features/relations/RelationListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'groups',
          lazy: () =>
            import('@/features/groups/GroupWorkspacePage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'governance',
          lazy: () =>
            import('@/features/governance/GovernancePage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'collaboration-audit',
          lazy: () =>
            import('@/features/audit/CollaborationAuditPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'extensions',
          lazy: () =>
            import('@/features/extensions/ExtensionListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'extensions/acceptance',
          lazy: () =>
            import('@/features/extensions/ExtensionAcceptancePage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'extensions/:id',
          lazy: () =>
            import('@/features/extensions/ExtensionDetailPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'templates',
          lazy: () =>
            import('@/features/templates/TemplateListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'templates/:id',
          lazy: () =>
            import('@/features/templates/TemplateDetailPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'usage',
          lazy: () => import('@/features/usage/UsageOverviewPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'usage/budgets',
          lazy: () => import('@/features/usage/UsageBudgetsPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'tools',
          lazy: () => import('@/features/tools/ToolListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'mcps',
          lazy: () => import('@/features/mcps/McpListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'skills',
          lazy: () =>
            import('@/features/skills/SkillListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'providers',
          lazy: () =>
            import('@/features/providers/ProviderListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'scenes',
          lazy: () =>
            import('@/features/scenes/SceneListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'knowledge',
          lazy: () =>
            import('@/features/knowledge/KnowledgeListPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'knowledge/:id',
          lazy: () =>
            import('@/features/knowledge/KnowledgeDetailPage').then((m) => ({ Component: m.default })),
          children: [
            { index: true, element: <Navigate to="documents" replace /> },
            {
              path: 'documents',
              lazy: () =>
                import('@/features/knowledge/KnowledgeDocumentsPage').then((m) => ({
                  Component: m.default
                }))
            },
            {
              path: 'documents/:documentId/chunks',
              lazy: () =>
                import('@/features/knowledge/KnowledgeChunksPage').then((m) => ({
                  Component: m.default
                }))
            },
            {
              path: 'retrieval',
              lazy: () =>
                import('@/features/knowledge/KnowledgeRetrievalPage').then((m) => ({
                  Component: m.default
                }))
            },
            {
              path: 'settings',
              lazy: () =>
                import('@/features/knowledge/KnowledgeSettingsPage').then((m) => ({
                  Component: m.default
                }))
            }
          ]
        },
        {
          path: 'chat',
          lazy: () =>
            import('@/features/chat/ChatViewPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'settings/cli-tokens',
          lazy: () =>
            import('@/features/cli-tokens/CLITokensPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'settings/aigc',
          lazy: () =>
            import('@/features/aigc-config/AigcConfigPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'settings/users',
          lazy: () => import('@/features/users/UsersPage').then((m) => ({ Component: m.default }))
        },
        {
          path: 'settings/audit-logs',
          lazy: () =>
            import('@/features/audit/AuditLogsPage').then((m) => ({ Component: m.default }))
        }
      ]
    },
    { path: '*', element: <NotFound /> }
  ],
  { basename: '/static/' }
)
