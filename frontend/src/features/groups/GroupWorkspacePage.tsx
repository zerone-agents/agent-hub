import { useMemo, useState } from 'react'
import { Alert, Button, Card, Empty, Form, Input, Modal, Select, Spin, Tabs, Tag } from 'antd'
import { createStyles } from 'antd-style'
import { PlusIcon } from '@phosphor-icons/react'
import PrimaryButton, { usePrimaryButtonStyle } from '@/components/PrimaryButton'
import { useCanWrite } from '@/hooks/useCanWrite'
import { useAgents } from '@/queries/useAgents'
import { groupApi, type ConversationSession, type GroupMemberRole, type SubscriptionMode } from '@/api/groups'
import { useChannelMessages, useChannelSessions, useChannelSubscriptions, useGroup, useGroupAction, useGroupAudit, useGroupChannels, useGroupMembers, useGroups } from '@/queries/useGroups'
import { tokens as t } from '@/styles/tokens'
import ExtensionSlotRenderer from '@/components/extensions/ExtensionSlotRenderer'

const useStyles = createStyles(({ css }) => ({
  page: css`animation: pageIn .3s ease; @keyframes pageIn { from { opacity: 0; transform: translateY(5px) } }`,
  header: css`display:flex; align-items:flex-start; justify-content:space-between; gap:20px; margin-bottom:20px; @media(max-width:720px){flex-direction:column}`,
  title: css`font-size:${t.text3xl}; font-weight:750; line-height:1.15; letter-spacing:-.03em; color:${t.text}; @media(max-width:600px){font-size:${t.text2xl}}`,
  subtitle: css`max-width:760px; margin-top:7px; color:${t.textTertiary}; font-size:${t.textBase}; line-height:1.65`,
  explainer: css`display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); margin-bottom:20px; border:1px solid color-mix(in srgb,var(--foreground) 8%,transparent); border-radius:${t.radius}px; overflow:hidden; background:${t.surface}; @media(max-width:700px){grid-template-columns:1fr}`,
  explainItem: css`padding:14px 16px; border-right:1px solid color-mix(in srgb,var(--foreground) 8%,transparent); color:${t.textSecondary}; line-height:1.5; &:last-child{border-right:0} strong{display:block;color:${t.text}} @media(max-width:700px){border-right:0;border-bottom:1px solid color-mix(in srgb,var(--foreground) 8%,transparent);&:last-child{border-bottom:0}}`,
  workspace: css`display:grid; grid-template-columns:minmax(230px,300px) minmax(0,1fr); min-height:620px; border:1px solid color-mix(in srgb,var(--foreground) 9%,transparent); border-radius:${t.radiusLg}px; overflow:hidden; background:${t.surface}; box-shadow:${t.elevation1}; @media(max-width:900px){grid-template-columns:1fr;min-height:0}`,
  rail: css`padding:16px; border-right:1px solid color-mix(in srgb,var(--foreground) 8%,transparent); background:color-mix(in srgb,${t.surface} 88%,${t.paper}); @media(max-width:900px){border-right:0;border-bottom:1px solid color-mix(in srgb,var(--foreground) 8%,transparent)}`,
  railTitle: css`font-weight:700; margin-bottom:12px`,
  groupList: css`display:flex; flex-direction:column; gap:7px; max-height:540px; overflow:auto; @media(max-width:900px){display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));max-height:260px}`,
  groupButton: css`border:1px solid transparent; border-radius:${t.radius}px; background:transparent; padding:12px; text-align:left; cursor:pointer; min-width:0; &:hover{background:${t.surfaceHover}}`,
  active: css`background:${t.inkSubtle}; border-color:color-mix(in srgb,${t.ink} 30%,transparent)`,
  groupName: css`font-weight:650; overflow:hidden; text-overflow:ellipsis; white-space:nowrap`,
  meta: css`margin-top:4px; color:${t.textMuted}; font-size:${t.textXs}`,
  detail: css`min-width:0;padding:22px 26px; @media(max-width:600px){padding:18px 14px}`,
  detailHead: css`display:flex;justify-content:space-between;gap:16px;align-items:flex-start;margin-bottom:12px; @media(max-width:600px){flex-direction:column}`,
  detailTitle: css`font-size:${t.textXl};font-weight:720`,
  cards: css`display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:12px`,
  card: css`border-color:color-mix(in srgb,var(--foreground) 9%,transparent); min-width:0; .ant-card-body{padding:15px}`,
  row: css`display:flex;align-items:center;justify-content:space-between;gap:10px;min-width:0; @media(max-width:480px){align-items:flex-start;flex-wrap:wrap}`,
  grow: css`min-width:0;flex:1`,
  name: css`font-weight:650;overflow-wrap:anywhere`,
  muted: css`color:${t.textMuted};font-size:${t.textSm};line-height:1.5`,
  toolbar: css`display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:14px;flex-wrap:wrap`,
  session: css`padding:14px;border:1px solid color-mix(in srgb,var(--foreground) 9%,transparent);border-radius:${t.radius}px;margin-bottom:10px`,
  history: css`display:flex;flex-direction:column;gap:10px`,
  historyItem: css`padding:11px 13px;border-left:3px solid ${t.ink};background:${t.inkSubtle};border-radius:0 ${t.radiusSm}px ${t.radiusSm}px 0`,
  empty: css`padding:70px 20px`,
}))

const roleOptions = [
  { value: 'leader', label: '负责人' }, { value: 'member', label: '成员' },
  { value: 'observer', label: '观察者' }, { value: 'guest', label: '访客' },
]
const subscribeLabel: Record<SubscriptionMode, string> = { all: '接收全部消息', mentions: '只接收提及', none: '不主动接收' }
const auditLabel: Record<string, string> = {
  group_created: '创建了群组', group_updated: '修改了群组', group_deleted: '删除了群组',
  member_added: '添加了成员', member_removed: '移除了成员', role_changed: '调整了成员角色',
  channel_created: '创建了频道', channel_updated: '修改了频道', channel_deleted: '删除了频道',
  subscription_upserted: '调整了频道订阅', session_created: '创建了会话房间',
  session_started: '开始了会话', session_completed: '结束会话并保存了总结',
}

export default function GroupWorkspacePage() {
  const { styles, cx } = useStyles()
  const primaryClass = usePrimaryButtonStyle()
  const canWrite = useCanWrite()
  const groupsQuery = useGroups()
  const agentsQuery = useAgents()
  const [selectedId, setSelectedId] = useState<string>()
  const actualId = selectedId ?? groupsQuery.data?.[0]?.id
  const detailQuery = useGroup(actualId)
  const auditQuery = useGroupAudit(actualId)
  const [dialog, setDialog] = useState<'group' | 'member' | 'channel' | 'session' | null>(null)
  const [selectedChannelId, setSelectedChannelId] = useState<string>()
  const [summarySession, setSummarySession] = useState<ConversationSession>()
  const [form] = Form.useForm()
  const [summaryForm] = Form.useForm()
  const group = detailQuery.data
  const membersQuery = useGroupMembers(actualId)
  const channelsQuery = useGroupChannels(actualId)
  const channels = channelsQuery.data ?? []
  const activeChannelId = selectedChannelId ?? channels[0]?.id
  const activeChannel = channels.find((item) => item.id === activeChannelId)
  const subscriptionsQuery = useChannelSubscriptions(activeChannelId)
  const sessionsQuery = useChannelSessions(activeChannelId)
  const messagesQuery = useChannelMessages(activeChannelId)
  const members = membersQuery.data ?? []
  const sessions = sessionsQuery.data ?? []

  const createGroup = useGroupAction((values: { name: string; description?: string; visibility: 'private' | 'tenant' }) => groupApi.create(values), '群组已创建')
  const addMember = useGroupAction((values: { agentId: number; role: GroupMemberRole }) => groupApi.addMember(actualId as string, values), '成员已加入群组')
  const addChannel = useGroupAction((values: { name: string; topic?: string; visibility: 'group' | 'members' }) => groupApi.addChannel(actualId as string, values), '频道已创建')
  const addSession = useGroupAction((values: { agenda: string; hostAgentId?: number; participantAgentIds?: number[] }) => groupApi.createSession(activeChannelId as string, values), '会话房间已创建')
  const updateMember = useGroupAction((input: { agentId: number; role: GroupMemberRole }) => groupApi.updateMember(actualId as string, input.agentId, input.role), '成员角色已更新')
  const updateSubscription = useGroupAction((input: { channelId: string; agentId: number; mode: SubscriptionMode }) => groupApi.updateSubscription(input.channelId, input.agentId, input.mode), '订阅方式已更新')
  const startSession = useGroupAction((id: string) => groupApi.startSession(id), '会话已开始')
  const completeSession = useGroupAction((input: { id: string; summary: string }) => groupApi.completeSession(input.id, input.summary), '会话已结束，结论已经留存')
  const agentOptions = useMemo(() => (agentsQuery.data ?? []).map((agent) => ({ value: agent.id, label: agent.config.title?.['zh-CN'] || agent.name })), [agentsQuery.data])
  const agentName = (id: number) => agentOptions.find((option) => option.value === id)?.label ?? `Agent #${id}`
  const memberOptions = members.map((member) => ({ value: member.agentId, label: agentName(member.agentId) }))

  const openDialog = (kind: typeof dialog) => { form.resetFields(); setDialog(kind) }
  const submitDialog = async () => {
    const values = await form.validateFields() as never
    if (dialog === 'group') await createGroup.mutateAsync(values)
    if (dialog === 'member') await addMember.mutateAsync(values)
    if (dialog === 'channel') await addChannel.mutateAsync(values)
    if (dialog === 'session') await addSession.mutateAsync(values)
    setDialog(null)
  }

  if (groupsQuery.isLoading) return <div className={styles.empty}><Spin /></div>
  return <div className={styles.page}>
    <div className={styles.header}>
      <div><div className={styles.title}>组织协作</div><div className={styles.subtitle}>让多个 Agent 在明确的成员范围和频道里共同工作。消息、订阅变化和会话结论都会留痕。</div></div>
      {canWrite && <PrimaryButton icon={<PlusIcon />} onClick={() => openDialog('group')}>新建群组</PrimaryButton>}
    </div>
    <div className={styles.explainer}>
      <div className={styles.explainItem}><strong>群组</strong>确定哪些 Agent 在同一个协作单元</div>
      <div className={styles.explainItem}><strong>频道</strong>把不同主题的群消息分开，并决定谁接收</div>
      <div className={styles.explainItem}><strong>会话房间</strong>围绕一个议题临时交流，结束时留下总结</div>
    </div>
    <div className={styles.workspace}>
      <aside className={styles.rail}>
        <div className={styles.railTitle}>群组</div>
        {!groupsQuery.data?.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有群组" /> : <div className={styles.groupList}>{groupsQuery.data.map((item) => <button type="button" key={item.id} className={cx(styles.groupButton, item.id === actualId && styles.active)} onClick={() => { setSelectedId(item.id); setSelectedChannelId(undefined) }}><div className={styles.groupName}>{item.name}</div><div className={styles.meta}>{item.visibility === 'tenant' ? '组织内可见' : '仅成员可见'}</div></button>)}</div>}
      </aside>
      <main className={styles.detail}>
        {!group ? <div className={styles.empty}><Empty description="选择一个群组查看协作空间" /></div> : <>
          <div className={styles.detailHead}><div><div className={styles.detailTitle}>{group.name}</div><div className={styles.muted}>{group.description || '暂无说明'}</div></div><Tag>{group.visibility === 'tenant' ? '组织内可见' : '仅成员可见'}</Tag></div>
          <Tabs items={[
            { key:'members', label:`成员 ${members.length}`, children:<><div className={styles.toolbar}><span className={styles.muted}>角色定义成员在团队中的职责；具体接收哪些消息由频道订阅决定。</span>{canWrite && <Button icon={<PlusIcon />} onClick={() => openDialog('member')}>添加成员</Button>}</div><div className={styles.cards}>{members.map((member) => <Card key={member.agentId} className={styles.card}><div className={styles.row}><div className={styles.grow}><div className={styles.name}>{agentName(member.agentId)}</div><div className={styles.muted}>Agent ID {member.agentId}</div></div><Select aria-label={`${agentName(member.agentId)}的角色`} value={member.role} options={roleOptions} disabled={!canWrite} style={{width:104}} onChange={(role) => updateMember.mutate({ agentId:member.agentId, role })} /></div></Card>)}</div>{!members.length && <Empty description="添加 Agent 后才能开展群组协作" />}</> },
            { key:'channels', label:`频道 ${channels.length}`, children:<><div className={styles.toolbar}><span className={styles.muted}>频道中的消息对相同成员可追溯。</span>{canWrite && <Button icon={<PlusIcon />} onClick={() => openDialog('channel')}>新建频道</Button>}</div><div className={styles.cards}>{channels.map((channel) => <Card key={channel.id} hoverable className={styles.card} onClick={() => setSelectedChannelId(channel.id)}><div className={styles.row}><div><div className={styles.name}># {channel.name}</div><div className={styles.muted}>{channel.topic || '暂无说明'}</div></div>{channel.messageCount !== undefined && <Tag>{channel.messageCount} 条消息</Tag>}</div><div className={styles.meta}>{channel.visibility === 'group' ? '群组成员可见' : '指定成员可见'}</div></Card>)}</div>{!channels.length && <Empty description="新建频道，开始多人协作" />}{activeChannel && <><h3># {activeChannel.name} · 订阅</h3><div className={styles.cards}>{members.map((member) => { const mode = subscriptionsQuery.data?.find((s) => s.agentId === member.agentId)?.mode ?? 'all'; return <Card key={member.agentId} className={styles.card}><div className={styles.row}><span className={styles.name}>{agentName(member.agentId)}</span><Select aria-label={`${agentName(member.agentId)}的订阅`} value={mode} disabled={!canWrite} style={{width:140}} options={Object.entries(subscribeLabel).map(([value,label]) => ({value,label}))} onChange={(next) => updateSubscription.mutate({channelId:activeChannel.id, agentId:member.agentId, mode:next})}/></div></Card>})}</div><h3>群消息历史</h3>{messagesQuery.data?.length ? <div className={styles.history}>{messagesQuery.data.map((msg) => <div className={styles.historyItem} key={msg.id}><div className={styles.row}><strong>{msg.sourceAgent ?? '系统'} → {msg.targetAgent ?? '频道'}</strong><Tag color={msg.status === 'completed' ? 'green' : msg.status === 'failed' ? 'red' : undefined}>{msg.status ?? '已记录'}</Tag></div><div>{msg.content}</div>{msg.reply && <div className={styles.muted}>回复：{msg.reply}</div>}{msg.error && <div className={styles.muted}>失败：{msg.error}</div>}<div className={styles.row}><span className={styles.muted}>{new Date(msg.createdAt).toLocaleString()}</span><span className={styles.muted}>{msg.verified ? `已核验：${msg.verification ?? '证据完整'}` : msg.verification ?? '等待核验'}</span></div></div>)}</div> : <Empty description="这里还没有群消息" />}</>}</> },
            { key:'sessions', label:'会话房间', children:<><Alert showIcon type="info" title="会话房间不是审批会议" description="它只是一个有议题、有群组成员、可结束并留存总结的临时讨论空间。"/><div className={styles.toolbar} style={{marginTop:14}}><span className={styles.muted}>先选择频道，再围绕议题发起会话。</span>{canWrite && <Button icon={<PlusIcon />} disabled={!activeChannel} onClick={() => openDialog('session')}>发起会话</Button>}</div>{activeChannel ? sessions.map((session) => <div className={styles.session} key={session.id}><div className={styles.row}><div className={styles.grow}><div className={styles.name}>{session.agenda}</div><div className={styles.muted}>主持人：{session.hostAgentName || (session.hostAgentId ? `Agent #${session.hostAgentId}` : '未指定')} · {session.participants?.length ?? 0} 位参会者</div></div><Tag color={session.status === 'active' ? 'green' : undefined}>{session.status === 'draft' ? '待开始' : session.status === 'active' ? '进行中' : '已结束'}</Tag></div>{session.participants?.length ? <div className={styles.meta}>参会快照：{session.participants.map((participant) => agentOptions.find((option) => option.value === participant.agentId)?.label ?? `Agent #${participant.agentId}`).join('、')}</div> : null}{session.summary && <p>{session.summary}</p>} {canWrite && session.status === 'draft' && <PrimaryButton onClick={() => startSession.mutate(session.id)}>开始会话</PrimaryButton>}{canWrite && session.status === 'active' && <Button onClick={() => setSummarySession(session)}>结束并总结</Button>}</div>) : <Empty description="请先创建或选择频道" />}{activeChannel && !sessions.length && <Empty description="这个频道还没有会话房间" />}</> },
            { key:'audit', label:'完整留痕', children:<><div className={styles.muted} style={{marginBottom:14}}>这里记录群组、成员、频道、订阅和会话的真实变化，不依赖 Agent 自己描述。</div>{auditQuery.data?.length ? <div className={styles.history}>{auditQuery.data.map((event) => <div className={styles.historyItem} key={event.id}><div className={styles.row}><strong>{event.description || `${auditLabel[event.action] ?? event.action}${event.agentId ? `：${agentName(event.agentId)}` : ''}`}</strong><span className={styles.muted}>{new Date(event.createdAt).toLocaleString()}</span></div>{event.resourceType && <div className={styles.muted}>对象：{event.resourceType} · {event.resourceId}</div>}{(event.actorName || event.actorId) && <div className={styles.muted}>操作人：{event.actorName || event.actorId}</div>}</div>)}</div> : <Empty description="暂无操作记录" />}</> },
          ]}/>
        </>}
      </main>
    </div>
    <Modal title={{group:'新建群组',member:'添加成员',channel:'新建频道',session:'发起会话'}[dialog ?? 'group']} open={Boolean(dialog)} onCancel={() => setDialog(null)} onOk={() => void submitDialog()} okText="确认" cancelText="取消" okButtonProps={{className:primaryClass.root}} destroyOnHidden>
      <Form form={form} layout="vertical" initialValues={{visibility: dialog === 'channel' ? 'group' : 'private', role:'member'}}>
        {dialog === 'group' && <><Form.Item label="群组名称" name="name" rules={[{required:true,message:'请输入群组名称'}]}><Input placeholder="例如：资产处置专项组" /></Form.Item><Form.Item label="群组说明" name="description"><Input.TextArea placeholder="这个群组共同负责什么？" /></Form.Item><Form.Item label="谁能看到" name="visibility"><Select options={[{value:'private',label:'仅成员可见'},{value:'tenant',label:'组织内可见'}]} /></Form.Item></>}
        {dialog === 'member' && <><Form.Item label="选择 Agent" name="agentId" rules={[{required:true,message:'请选择 Agent'}]}><Select showSearch optionFilterProp="label" options={agentOptions}/></Form.Item><Form.Item label="群组角色" name="role"><Select options={roleOptions}/></Form.Item></>}
        {dialog === 'channel' && <><Form.Item label="频道名称" name="name" rules={[{required:true,message:'请输入频道名称'}]}><Input placeholder="例如：风险评估" /></Form.Item><Form.Item label="频道用途" name="topic"><Input.TextArea placeholder="这个频道讨论什么？" /></Form.Item><Form.Item label="消息可见范围" name="visibility"><Select options={[{value:'group',label:'全体群组成员'},{value:'members',label:'指定成员'}]}/></Form.Item></>}
        {dialog === 'session' && <><Form.Item label="议题" name="agenda" rules={[{required:true,message:'请输入本次讨论的议题'}]}><Input placeholder="例如：是否暂停资产出售计划" /></Form.Item><Form.Item label="主持人（可选）" name="hostAgentId"><Select allowClear options={memberOptions}/></Form.Item><Form.Item label="参会者" name="participantAgentIds" extra="不选择时，将使用频道当前接收者并保存为参会快照。"><Select mode="multiple" allowClear options={memberOptions}/></Form.Item></>}
      </Form>
    </Modal>
    <ExtensionSlotRenderer slot="group.detail.tab" />
    <Modal title="结束会话" open={Boolean(summarySession)} onCancel={() => setSummarySession(undefined)} okText="结束并保存" cancelText="取消" okButtonProps={{className:primaryClass.root}} onOk={() => void summaryForm.validateFields().then(({summary}:{summary:string}) => completeSession.mutate({id:summarySession?.id as string,summary}, {onSuccess:()=>{ summaryForm.resetFields(); setSummarySession(undefined) }}))}><p className={styles.muted}>请留下本次讨论的结论，方便其他 Agent 和管理员复盘。</p><Form form={summaryForm}><Form.Item name="summary" rules={[{required:true,message:'请输入会话总结'}]}><Input.TextArea rows={5} placeholder="本次讨论达成了什么结论？下一步是什么？" /></Form.Item></Form></Modal>
  </div>
}
