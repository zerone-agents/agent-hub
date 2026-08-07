import { createBrowserRouter, Navigate } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import MainLayout from '@/layouts/MainLayout'
import NotFound from '@/components/NotFound'
import AgentChatPage from '@/features/agent-chat/AgentChatPage'

export const router = createBrowserRouter(
  [
    {
      path: '/login',
      lazy: () => import('@/features/login/LoginPage').then((m) => ({ Component: m.default }))
    },
    {
      path: '/agents/:name/chat',
      element: (
        <RequireAuth>
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
        }
      ]
    },
    { path: '*', element: <NotFound /> }
  ],
  { basename: '/static/' }
)
