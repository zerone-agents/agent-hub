import React, { useState, useEffect, useRef } from 'react';
import { ActiveTab, Agent, ChatMessage, KnowledgeDocument, KnowledgeFolder, ScenarioPrompt } from './types';
import {
  INITIAL_AGENTS,
} from './data/mockData';
import { MobileFrame } from './components/MobileFrame';
import { BottomNav } from './components/BottomNav';
import { HomeChatView } from './components/HomeChatView';
import { AgentsView } from './components/AgentsView';
import { AgentDetailModal } from './components/AgentDetailModal';
import { KnowledgeBaseView } from './components/KnowledgeBaseView';
import type { KnowledgeFolderFormInput } from './components/KnowledgeBaseModal';
import { DocumentModal } from './components/DocumentModal';
import { UploadDocumentModal } from './components/UploadDocumentModal';
import { ProfileView } from './components/ProfileView';
import {
  listDatasets,
  createDataset,
  updateDataset,
  deleteDatasets,
  listDocuments,
  completeUpload,
  ingestDocuments,
  stopParsingDocuments,
  KnowledgeConfigurationPendingError,
  type UploadResume,
  type MobileChunk,
  renameDocument,
  deleteDocuments,
} from './api/knowledge';
import { fetchPublicAgents, ApiError } from './api/agents';
import { fetchScenes } from './api/scenes';
import { createChatSession, streamChatMessage } from './api/chat';
import { getStoredAuth, storeAuth, loginWithToken, extractOAuthTokensFromUrl, AUTH_CHANGED_EVENT, fetchAuthMode, OAUTH_REDIRECT_PATH } from './api/auth';
import type { AuthMode } from './api/auth';
import { LoginGuide } from './components/LoginGuide';

/** 默认 Agent：优先「办公助手」（暂时按名称硬编码，后续可改为后台配置），
 *  找不到再退列表第一个。绝不再落到本地假数据。 */
function pickDefaultAgent(list: Agent[]): Agent | null {
  if (!list.length) return null;
  return (
    list.find((a) => a.id.includes('office') || a.name.includes('办公')) ?? list[0]
  );
}

export default function App() {
  const [activeTab, setActiveTab] = useState<ActiveTab>('chat');

  // 当前登录角色（未登录为 undefined）。
  // guest（体验用户）：后端 /api/v1/admin/** 恒 403 → 知识库 Tab 对其隐藏（zerone guest 定位=仅聊天）。
  const [authRole, setAuthRole] = useState<string | undefined>(() => getStoredAuth()?.role);
  const showKnowledge = authRole !== 'guest';
  // 知识库写操作（新建/上传/编辑/删除）：后端 /api/v1/admin/** 需 maintainer+，
  // member/guest 只能看（guest 连 Tab 都不显示，见上）。
  const canWriteKnowledge = authRole === 'admin' || authRole === 'maintainer';

  // 登录入口：casdoor（线上）直接跳 /auth/login（302 到 Casdoor 授权页，PKCE 参数由后端动态生成），
  // 不经过「我的」中转页；builtin（本地 mock）落到「我的」页账号密码登录。
  const goLogin = () => {
    fetchAuthMode().then((mode) => {
      if (mode === 'casdoor') {
        window.location.href = `/auth/login?redirect=${encodeURIComponent(OAUTH_REDIRECT_PATH)}`;
      } else {
        setActiveTab('profile');
      }
    });
  };

  // 后端认证模式（casdoor=线上 SSO / builtin=本地 mock 账号密码）
  const [authMode, setAuthMode] = useState<AuthMode | null>(null);
  useEffect(() => {
    fetchAuthMode().then(setAuthMode);
  }, []);

  // Casdoor OAuth 回调落地：token 走 URL fragment（#token=&refreshToken=，issue #185 起；
  // 旧 query 格式滚动兼容），部署在 console /static/h5/ 时由 /auth/login?redirect=/h5/
  // 完成跳转。验证 token 后写入登录态并广播。
  useEffect(() => {
    const tokens = extractOAuthTokensFromUrl();
    if (!tokens) return;
    loginWithToken(tokens.token, tokens.refreshToken)
      .then((a) => {
        storeAuth(a);
        setAuthRole(a.role);
        window.dispatchEvent(new Event(AUTH_CHANGED_EVENT));
        if (a.role === 'guest') setActiveTab('chat');
      })
      .catch(() => {
        // token 无效：静默忽略，用户可在「我的」重新登录
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 知识库只显示当前身份的后端结果，不从跨账号缓存或示例数据恢复。
  const [documents, setDocuments] = useState<KnowledgeDocument[]>([]);
  const [folders, setFolders] = useState<KnowledgeFolder[]>([]);
  const [knowledgeRevision, setKnowledgeRevision] = useState(0);

  // Agents state —— 从 agent-hub 对客公开接口（/api/v1/agents?view=chat）拉取。
  // 真实后端整组要求 JWT：401 时绝不显示本地兜底假数据，而是引导登录；
  // 只有网络/5xx 等接口不可达时才保留 INITIAL_AGENTS 兜底（离线开发用）。
  const [agents, setAgents] = useState<Agent[]>(INITIAL_AGENTS);
  const [agentsNeedLogin, setAgentsNeedLogin] = useState(false);
  // 场景（预设问题卡片）：来自 agent-hub GET /api/v1/scenes，按 agent 名分组。
  // 首页卡片只显示当前 Agent 的场景；没有就不显示，不用写死的假场景。
  const [scenes, setScenes] = useState<ScenarioPrompt[]>([]);

  // 拉取 Agent 列表（function 声明提升，供下面两个 effect 复用）。
  // isStale 用于忽略过期响应（effect 清理后不再 setState）。
  // 拉取期间保留旧列表，拉到新数据再整体替换——切 Tab 重新拉时不闪空。
  function loadAgents(isStale?: () => boolean) {
    fetchPublicAgents()
      .then((list) => {
        if (isStale?.()) return;
        setAgentsNeedLogin(false);
        setAgents(list); // 以线上真实数据为准，空列表就是空
        // 恢复上次选择：仅当它还存在于真实列表时生效，否则默认「办公助手」
        const savedId = savedAgentIdRef.current;
        const restored =
          savedId && savedId !== 'general'
            ? list.find((a) => a.id === savedId)
            : null;
        setActiveAgent(restored ?? pickDefaultAgent(list));
      })
      .catch((err) => {
        if (isStale?.()) return;
        if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
          setAgents([]);
          setActiveAgent(null);
          setAgentsNeedLogin(true);
        }
        // 其他错误（网络/5xx）：静默保留现有列表兜底
      });
  }

  useEffect(() => {
    let cancelled = false;
    loadAgents(() => cancelled);
    fetchScenes()
      .then((list) => {
        if (!cancelled) setScenes(list);
      })
      .catch(() => {
        // 场景拉不到就空着，卡片自然隐藏
      });
    return () => {
      cancelled = true;
    };
    // authRole 变化（登录/退出）后重新拉取，保证列表跟随身份切换
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authRole]);

  // 每次进「专家」Tab 重新拉一次 Agent 列表：后台改了激活开关后切过来就能看到，
  // 不用整页刷新。拉取期间旧列表还在，拉到再替换（loadAgents 内部保证不闪空）。
  useEffect(() => {
    if (activeTab !== 'agents') return;
    loadAgents();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab]);

  // Active chat state
  const [messages, setMessages] = useState<ChatMessage[]>(() => {
    try {
      const saved = localStorage.getItem('workbuddy_chat_messages');
      if (saved) return JSON.parse(saved);
    } catch {
      // ignore
    }
    return [];
  });

  // 上次选中的 Agent id：先记下来，等真实列表回来后按列表匹配恢复
  //（不再从本地假数据 INITIAL_AGENTS 恢复，避免默认落到已删掉的假 Agent 上）
  const savedAgentIdRef = useRef<string | null>(
    (() => {
      try {
        return localStorage.getItem('workbuddy_active_agent_id');
      } catch {
        return null;
      }
    })()
  );
  const [activeAgent, setActiveAgent] = useState<Agent | null>(null);

  // Keep active agent synced to localStorage
  useEffect(() => {
    savedAgentIdRef.current = activeAgent ? activeAgent.id : 'general';
    if (activeAgent) {
      localStorage.setItem('workbuddy_active_agent_id', activeAgent.id);
    } else {
      localStorage.setItem('workbuddy_active_agent_id', 'general');
    }
  }, [activeAgent]);

  // 当前 Agent 的预设场景卡片（未选 Agent 时用列表第一个，与发送兜底逻辑一致）
  const chatAgent = activeAgent ?? agents[0] ?? null;
  const activeScenes = chatAgent
    ? scenes.filter((s) => s.category === chatAgent.id).map((s) => ({ ...s, icon: chatAgent.avatar }))
    : [];

  const [isLoading, setIsLoading] = useState(false);

  // Modals state
  const [selectedAgentDetail, setSelectedAgentDetail] = useState<Agent | null>(null);
  const [uploadModalState, setUploadModalState] = useState<{
    isOpen: boolean;
    targetFolderId?: string | null;
  }>({
    isOpen: false,
    targetFolderId: null,
  });
  const [docModalState, setDocModalState] = useState<{
    isOpen: boolean;
    mode: 'view' | 'edit' | 'upload';
    doc: KnowledgeDocument | null;
    chunk?: MobileChunk;
  }>({
    isOpen: false,
    mode: 'view',
    doc: null,
  });

  useEffect(() => {
    try {
      localStorage.setItem('workbuddy_chat_messages', JSON.stringify(messages));
    } catch {
      // ignore
    }
  }, [messages]);

  const [parsingAcceptedAt, setParsingAcceptedAt] = useState(0);
  const [knowledgeError, setKnowledgeError] = useState('');
  const [knowledgeLoading, setKnowledgeLoading] = useState(false);
  const refreshKnowledge = async () => {
    const token = getStoredAuth()?.token;
    const remoteFolders = await listDatasets();
    const groups = await Promise.all(remoteFolders.map(f => listDocuments(f.id, f.name)));
    if (token !== getStoredAuth()?.token) throw new Error('登录身份已变化，请重新加载知识库');
    setFolders(remoteFolders); setDocuments(groups.flat()); setKnowledgeError('');
  };
  useEffect(() => {
    const changed = (event: Event) => {
      if (event instanceof StorageEvent && event.key !== 'zerone_auth') return;
      setAuthRole(getStoredAuth()?.role);
      setKnowledgeRevision(value => value + 1);
      setDocuments([]); setFolders([]);
      setUploadModalState({ isOpen: false });
      setDocModalState({ isOpen: false, mode: 'view', doc: null });
    };
    window.addEventListener(AUTH_CHANGED_EVENT, changed);
    window.addEventListener('storage', changed);
    return () => {
      window.removeEventListener(AUTH_CHANGED_EVENT, changed);
      window.removeEventListener('storage', changed);
    };
  }, []);
  useEffect(() => {
    let cancelled = false;
    setDocuments([]); setFolders([]); setKnowledgeError(''); setKnowledgeLoading(false);
    setUploadModalState({ isOpen: false });
    setDocModalState({ isOpen: false, mode: 'view', doc: null });
    if (!getStoredAuth() || authRole === 'guest') return;
    setKnowledgeLoading(true);
    const token = getStoredAuth()?.token;
    (async () => {
      try {
        const remoteFolders = await listDatasets();
        const groups = await Promise.all(remoteFolders.map(f => listDocuments(f.id, f.name)));
        if (cancelled || token !== getStoredAuth()?.token) return;
        setFolders(remoteFolders); setDocuments(groups.flat());
      } catch (err) {
        if (!cancelled) setKnowledgeError(err instanceof Error ? err.message : '知识库加载失败');
      } finally { if (!cancelled) setKnowledgeLoading(false); }
    })();
    return () => { cancelled = true; };
  }, [authRole, knowledgeRevision]);
  useEffect(() => {
    if (activeTab !== 'knowledge' || (!documents.some(doc => doc.run === '1') && Date.now() - parsingAcceptedAt >= 60000)) return;
    const timer = window.setTimeout(() => {
      refreshKnowledge().catch(err => setKnowledgeError(err instanceof Error ? err.message : '状态刷新失败'));
    }, 5000);
    return () => window.clearTimeout(timer);
  }, [activeTab, documents, parsingAcceptedAt]);

  const handleCreateFolder = async (data: KnowledgeFolderFormInput) => {
    try {
      const folder = await createDataset(data.name, data.description, data.existingId, data.configuration);
      setFolders(prev => [folder, ...prev.filter(f => f.id !== folder.id)]);
      return folder;
    } catch (err) {
      if (err instanceof KnowledgeConfigurationPendingError) {
        setFolders(prev => [err.folder, ...prev.filter(f => f.id !== err.folder.id)]);
      }
      throw err;
    }
  };
  const handleUpdateFolder = async (folderId: string, data: { name: string; description: string; parseMethod: string; category: 'mine' | 'team' }) => {
    const folder = await updateDataset(folderId, { name: data.name, description: data.description });
    setFolders(prev => prev.map(f => f.id === folderId ? folder : f));
    setDocuments(prev => prev.map(doc => doc.folderId === folderId ? { ...doc, folderName: folder.name } : doc));
  };
  const handleDeleteFolder = async (folderId: string) => {
    await deleteDatasets([folderId]);
    setFolders(prev => prev.filter(f => f.id !== folderId));
    setDocuments(prev => prev.filter(doc => doc.folderId !== folderId));
  };
  const handleUploadFiles = async (files: File[], folderId: string, autoParse: boolean, resume?: UploadResume) => {
    const folder = folders.find(f => f.id === folderId);
    if (!folder) throw new Error('请选择已有知识库');
    await completeUpload(folderId, folder.name, files, autoParse, created => {
      const ids = new Set(created.map(doc => doc.id));
      setDocuments(prev => [...created, ...prev.filter(doc => !ids.has(doc.id))]);
    }, refreshKnowledge, resume);
  };

  // 聊天会话 id 缓存：key = agent 的 hub name（即 agent.id），value = 会话 id
  const sessionIdsRef = useRef<Record<string, string>>({});
  const sessionsLoadedRef = useRef(false);
  if (!sessionsLoadedRef.current) {
    sessionsLoadedRef.current = true;
    try {
      sessionIdsRef.current = JSON.parse(
        localStorage.getItem('workbuddy_chat_sessions') || '{}'
      ) as Record<string, string>;
    } catch {
      // ignore
    }
  }

  // Send message handler —— 走 agent-hub 对客聊天接口（会话 + SSE 流式），
  // 与 /agents/chat 页面同一套后端。会话 id 按 agent 维度缓存（localStorage），
  // 同一 agent 的多轮对话在同一个会话里累积上下文。
  const handleSendMessage = async (text: string, agentOverride?: Agent) => {
    const agent = agentOverride ?? activeAgent ?? pickDefaultAgent(agents);
    const userMsgId = 'msg-' + Date.now();
    const userMessage: ChatMessage = {
      id: userMsgId,
      role: 'user',
      content: text,
      timestamp: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
    };

    setMessages((prev) => [...prev, userMessage]);
    setIsLoading(true);

    const assistantMsgId = 'msg-' + (Date.now() + 1);
    const baseAssistant: ChatMessage = {
      id: assistantMsgId,
      role: 'assistant',
      content: '',
      timestamp: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
      agentId: agent?.id,
      agentName: agent ? agent.name : 'Zerone AI 搭档',
      agentAvatar: agent ? agent.avatar : '🤖',
      isStreaming: true,
    };

    const patchAssistant = (patch: Partial<ChatMessage>) => {
      setMessages((prev) =>
        prev.map((m) => (m.id === assistantMsgId ? { ...m, ...patch } : m))
      );
    };

    // agent 列表还没从接口回来（或本地兜底数据）时，agent.id 不是 hub name，
    // 直接提示，不打必失败的请求。
    if (!agent) {
      setMessages((prev) => [
        ...prev,
        { ...baseAssistant, content: '专家列表尚未加载完成，请稍后重试。', isStreaming: false },
      ]);
      setIsLoading(false);
      return;
    }

    try {
      // 1. 确保会话存在（按 agent 缓存，不存在则新建）
      let sessionId = sessionIdsRef.current[agent.id];
      if (!sessionId) {
        const session = await createChatSession(agent.id, text.slice(0, 20));
        sessionId = session.id;
        sessionIdsRef.current = { ...sessionIdsRef.current, [agent.id]: sessionId };
        try {
          localStorage.setItem('workbuddy_chat_sessions', JSON.stringify(sessionIdsRef.current));
        } catch {
          // ignore
        }
      }

      // 2. 追加流式占位气泡，边收边更新
      setMessages((prev) => [...prev, baseAssistant]);

      await streamChatMessage(agent.id, sessionId, text, {
        onDelta: (fullText) => patchAssistant({ content: fullText }),
        onDone: (fullText) => {
          patchAssistant({ content: fullText || '（无内容返回）', isStreaming: false });
          setIsLoading(false);
        },
        onError: (message) => {
          patchAssistant({ content: `⚠️ ${message}`, isStreaming: false });
          setIsLoading(false);
        },
      });
    } catch (err) {
      // 建会话等前置步骤失败
      setMessages((prev) => [
        ...prev,
        {
          ...baseAssistant,
          content: `⚠️ 发送失败：${err instanceof Error ? err.message : '未知错误'}`,
          isStreaming: false,
        },
      ]);
      setIsLoading(false);
    }
  };

  // 切换 Agent = 切换对话窗口：不同 Agent 是不同会话，
  // 切换时清空当前消息列表（各 Agent 的会话 id 已按 agent 缓存，历史仍在后端）。
  const handleSelectAgent = (agent: Agent) => {
    if (agent.id === activeAgent?.id) return;
    setActiveAgent(agent);
    setMessages([]);
  };

  // Summon agent into chat
  const handleSummonAgent = (agent: Agent, promptText?: string) => {
    handleSelectAgent(agent);
    setSelectedAgentDetail(null);
    setActiveTab('chat');

    if (promptText) {
      // 显式传 agent，绕开 setActiveAgent 的异步时序
      handleSendMessage(promptText, agent);
    }
  };

  // 文档编辑只提交已有文档的名称；内容修改需通过切片页面。
  const handleSaveDocument = async (docData: Partial<KnowledgeDocument>) => {
    const existing = documents.find(doc => doc.id === docData.id);
    if (!existing?.folderId || !docData.name?.trim()) throw new Error('请选择已有文档');
    await renameDocument(existing.folderId, existing.id, docData.name.trim());
    setDocuments(prev => prev.map(doc => doc.id === existing.id ? { ...doc, name: docData.name.trim() } : doc));
  };
  const handleDeleteDocument = async (id: string) => {
    const existing = documents.find(doc => doc.id === id);
    if (!existing?.folderId) throw new Error('文档不存在，请刷新');
    await deleteDocuments(existing.folderId, [id]);
    setDocuments(prev => prev.filter(doc => doc.id !== id));
    // 数量由后端回读；刷新失败保留删除结果并展示错误。
    try { await refreshKnowledge(); }
    catch (err) { setKnowledgeError(err instanceof Error ? err.message : '知识库状态刷新失败'); }
  };

  const handleDocumentParsing = async (id: string, stop: boolean, options = { delete: false, apply_kb: false }) => {
    const existing = documents.find(doc => doc.id === id);
    if (!existing?.folderId) throw new Error('文档不存在，请刷新');
    if (stop) await stopParsingDocuments(existing.folderId, [id]);
    else { await ingestDocuments([id], options); setParsingAcceptedAt(Date.now()); }
    try { await refreshKnowledge(); }
    catch (err) { setKnowledgeError(err instanceof Error ? err.message : '状态刷新失败'); }
  };

  // 未登录判定：本地无登录态（authRole 为空）或接口 401（token 过期，agentsNeedLogin）。
  // casdoor（线上）模式下未登录 → 所有 Tab 统一展示同一套登录引导页（含知识库——之前它没有提示），
  // 登录按钮直跳 Casdoor SSO，不经过「我的」中转。builtin（本地 mock）保持各 Tab 原有引导。
  const needAuth = !authRole || agentsNeedLogin;
  const showUnifiedLogin = authMode === 'casdoor' && needAuth;

  return (
    <MobileFrame>
      {/* View Switcher based on active tab */}
      <div className="flex-1 flex flex-col min-h-0 relative overflow-hidden">
        {showUnifiedLogin ? (
          <LoginGuide
            onSsoLogin={goLogin}
            onAuthSuccess={(a) => {
              setAuthRole(a.role);
              setKnowledgeRevision(value => value + 1);
              // 体验用户（guest）登录后自动跳聊天页
              if (a.role === 'guest') setActiveTab('chat');
            }}
          />
        ) : (
        <>
        {activeTab === 'chat' && (
          <HomeChatView
            scenarioPrompts={activeScenes}
            messages={messages}
            activeAgent={activeAgent}
            allAgents={agents}
            needLogin={agentsNeedLogin}
            isLoading={isLoading}
            onSendMessage={handleSendMessage}
            onSelectAgent={handleSelectAgent}
            onNavigateToTab={(tab) => setActiveTab(tab)}
            onGoLogin={goLogin}
          />
        )}

        {activeTab === 'agents' && (
          <AgentsView
            agents={agents}
            needLogin={agentsNeedLogin}
            onGoLogin={goLogin}
            onSelectAgent={(agent) => setSelectedAgentDetail(agent)}
          />
        )}

        {activeTab === 'knowledge' && showKnowledge && (
          <KnowledgeBaseView
            key={`${authRole}-${knowledgeRevision}`}
            loading={knowledgeLoading}
            error={knowledgeError}
            onRefresh={async () => { setKnowledgeLoading(true); try { await refreshKnowledge(); } finally { setKnowledgeLoading(false); } }}
            documents={documents}
            folders={folders}
            canWrite={canWriteKnowledge}
            onOpenUpload={(folderId) =>
              setUploadModalState({
                isOpen: true,
                targetFolderId: folderId || null,
              })
            }
            onOpenDocDetail={(doc, chunk) =>
              setDocModalState({
                isOpen: true,
                mode: 'view',
                doc,
                chunk,
              })
            }
            onOpenDocEdit={(doc) =>
              setDocModalState({
                isOpen: true,
                mode: 'edit',
                doc,
              })
            }
            onDeleteDoc={handleDeleteDocument}
            onCreateFolder={handleCreateFolder}
            onUpdateFolder={handleUpdateFolder}
            onDeleteFolder={handleDeleteFolder}
          />
        )}

        {activeTab === 'profile' && (
          <ProfileView
            onAuthSuccess={(a) => {
              setAuthRole(a.role);
              setKnowledgeRevision(value => value + 1);
              // 体验用户（guest）登录/注册后自动跳转聊天体验页
              if (a.role === 'guest') setActiveTab('chat');
            }}
            onLogout={() => { setAuthRole(undefined); setKnowledgeRevision(value => value + 1); }}
          />
        )}
        </>
        )}

        {/* Agent Detail Modal (matching Screenshot 4) */}
        <AgentDetailModal
          agent={selectedAgentDetail}
          isOpen={Boolean(selectedAgentDetail)}
          onClose={() => setSelectedAgentDetail(null)}
          onSummon={handleSummonAgent}
        />

        {/* Upload Document Modal (matching Image 4) */}
        <UploadDocumentModal
          isOpen={uploadModalState.isOpen}
          folders={folders}
          defaultFolderId={uploadModalState.targetFolderId}
          onClose={() =>
            setUploadModalState({
              isOpen: false,
              targetFolderId: null,
            })
          }
          onUploadFiles={handleUploadFiles}
        />

        {/* Document Modal (View, Edit) */}
        <DocumentModal
          mode={docModalState.mode}
          document={documents.find(doc => doc.id === docModalState.doc?.id) ?? docModalState.doc}
          folders={folders}
          initialChunk={docModalState.chunk}
          isOpen={docModalState.isOpen}
          onClose={() =>
            setDocModalState({
              isOpen: false,
              mode: 'view',
              doc: null,
            })
          }
          canWrite={canWriteKnowledge}
          onRefresh={refreshKnowledge}
          onParse={(id, options) => handleDocumentParsing(id, false, options)}
          onStop={id => handleDocumentParsing(id, true)}
          onSave={handleSaveDocument}
          onDelete={handleDeleteDocument}
        />
      </div>

      {/* Bottom Navigation Bar */}
      <BottomNav
        activeTab={activeTab}
        onChangeTab={(tab) => setActiveTab(tab)}
        showKnowledge={showKnowledge}
        knowledgeCount={documents.length}
      />
    </MobileFrame>
  );
}
