import { useMemo, useState } from "react";
import {
  Alert,
  Button,
  Checkbox,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Progress,
  Select,
  Spin,
  Tabs,
  Tag,
  Timeline,
  message,
} from "antd";
import { createStyles } from "antd-style";
import { CheckCircleIcon, PlusIcon, WarningIcon } from "@phosphor-icons/react";
import PrimaryButton, {
  usePrimaryButtonStyle,
} from "@/components/PrimaryButton";
import { unwrapResponse } from "@/api/client";
import {
  workflowApi,
  type WorkflowStepDefinition,
  type WorkflowExecution,
} from "@/api/workflows";
import {
  decisionApi,
  type CollectiveDecision,
  type DecisionChoice,
} from "@/api/decisions";
import type { Agent } from "@/api/agents";
import type {
  CollaborationGroup,
  GroupMember,
  GroupMemberRole,
} from "@/api/groups";
import {
  useDecision,
  useDecisionAudit,
  useDecisions,
  useGovernanceAction,
  useWorkflow,
  useWorkflowExecution,
  useWorkflowExecutions,
  useWorkflows,
} from "@/queries/useGovernance";
import { useGroups, useGroupMembers } from "@/queries/useGroups";
import { useAgents } from "@/queries/useAgents";
import { useCanWrite } from "@/hooks/useCanWrite";
import { tokens as t } from "@/styles/tokens";

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.3s ease;
    @keyframes pageIn {
      from {
        opacity: 0;
        transform: translateY(5px);
      }
    }
  `,
  head: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 20px;
    @media (max-width: 700px) {
      flex-direction: column;
    }
  `,
  title: css`
    font-size: ${t.text3xl};
    font-weight: 760;
    letter-spacing: -0.035em;
    line-height: 1.15;
    @media (max-width: 600px) {
      font-size: ${t.text2xl};
    }
  `,
  sub: css`
    max-width: 780px;
    margin-top: 7px;
    color: ${t.textTertiary};
    line-height: 1.65;
  `,
  ledger: css`
    display: grid;
    grid-template-columns: minmax(250px, 330px) minmax(0, 1fr);
    min-height: 640px;
    border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: ${t.radiusLg}px;
    overflow: hidden;
    background: ${t.surface};
    box-shadow: ${t.elevation1};
    @media (max-width: 920px) {
      grid-template-columns: 1fr;
      min-height: 0;
    }
  `,
  rail: css`
    padding: 17px;
    background: color-mix(in srgb, ${t.surface} 88%, ${t.paper});
    border-right: 1px solid
      color-mix(in srgb, var(--foreground) 8%, transparent);
    @media (max-width: 920px) {
      border-right: 0;
      border-bottom: 1px solid
        color-mix(in srgb, var(--foreground) 8%, transparent);
    }
  `,
  detail: css`
    min-width: 0;
    padding: 22px 26px;
    @media (max-width: 600px) {
      padding: 16px 14px;
    }
  `,
  list: css`
    display: flex;
    flex-direction: column;
    gap: 7px;
    max-height: 560px;
    overflow: auto;
    @media (max-width: 920px) {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      max-height: 260px;
    }
  `,
  item: css`
    width: 100%;
    padding: 13px;
    border: 1px solid transparent;
    border-radius: ${t.radius}px;
    background: transparent;
    text-align: left;
    cursor: pointer;
    &:hover {
      background: ${t.surfaceHover};
    }
  `,
  active: css`
    background: ${t.inkSubtle};
    border-color: color-mix(in srgb, ${t.ink} 28%, transparent);
  `,
  row: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    min-width: 0;
    @media (max-width: 480px) {
      align-items: flex-start;
      flex-wrap: wrap;
    }
  `,
  name: css`
    font-weight: 680;
    overflow-wrap: anywhere;
  `,
  muted: css`
    color: ${t.textMuted};
    font-size: ${t.textSm};
    line-height: 1.5;
  `,
  stageLine: css`
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr) auto;
    gap: 10px;
    align-items: center;
    padding: 14px 0;
    border-bottom: 1px solid
      color-mix(in srgb, var(--foreground) 7%, transparent);
    @media (max-width: 520px) {
      grid-template-columns: 28px minmax(0, 1fr);
      > :last-child {
        grid-column: 2;
      }
    }
  `,
  stageDot: css`
    width: 26px;
    height: 26px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: ${t.inkSubtle};
    color: ${t.ink};
    font-size: 12px;
    font-weight: 700;
  `,
  stats: css`
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
    margin: 16px 0;
    @media (max-width: 560px) {
      grid-template-columns: 1fr;
    }
  `,
  stat: css`
    padding: 13px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radius}px;
    .value {
      font-size: ${t.textXl};
      font-weight: 720;
    }
  `,
  actions: css`
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 12px;
  `,
  examples: css`
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin: 12px 0 18px;
  `,
  section: css`
    margin: 18px 0 10px;
    font-size: ${t.textLg};
    font-weight: 700;
  `,
  empty: css`
    padding: 70px 20px;
  `,
}));

type Example = "content" | "asset";
const exampleCopy = {
  content: {
    label: "内容发布审核",
    name: "内容发布标准流程",
    description: "作者提交后，由事实审核和法务并行检查，最后由负责人批准。",
  },
  asset: {
    label: "重大资产处置",
    name: "资产处置决策流程",
    description: "财务预演、法务审核、决策表决和合规确认依次留痕。",
  },
};
type ExampleValues = {
  groupId: string;
  agentIds: number[];
  approvalRole: "leader" | "member" | "observer" | "guest";
  startNow?: boolean;
};
export const buildExampleSteps = (
  kind: Example,
  values: ExampleValues,
  agentNames: Map<number, string>,
  roles: Map<number, string>,
): WorkflowStepDefinition[] => {
  const names = values.agentIds
    .map((id) => agentNames.get(id))
    .filter((name): name is string => Boolean(name));
  if (!names.length) throw new Error("请选择至少一位当前群组成员");
  const actor = (index: number) => ({
    actorType: "agent",
    actorRef: names[index % names.length],
  });
  if (kind === "content")
    return [
      { key: "draft", name: "提交初稿", type: "task", ...actor(0) },
      {
        key: "facts",
        name: "事实核验",
        type: "task",
        ...actor(1),
        dependsOn: ["draft"],
      },
      {
        key: "legal",
        name: "风险复核",
        type: "task",
        ...actor(2),
        dependsOn: ["draft"],
      },
      {
        key: "approve",
        name: "负责人确认",
        type: "approval",
        actorType: "role",
        actorRef: values.approvalRole,
        dependsOn: ["facts", "legal"],
        config: { groupId: values.groupId, policy: "all" },
        timeoutSeconds: 86400,
      },
    ];
  const electors = Array.from(roles.entries()).map(([agentId, role]) => ({
    agentId,
    weight: 1,
    canVeto: role === "leader",
  }));
  return [
    { key: "finance", name: "方案测算", type: "task", ...actor(0) },
    {
      key: "legal",
      name: "风险复核",
      type: "task",
      ...actor(1),
      dependsOn: ["finance"],
    },
    {
      key: "decision",
      name: "群体表决",
      type: "decision",
      actorType: "group",
      actorRef: values.groupId,
      dependsOn: ["legal"],
      config: {
        groupId: values.groupId,
        title: "是否通过重大事项方案",
        description: "基于测算和风险复核结果形成集体决定",
        quorumPercent: 60,
        approvalPercent: 50,
        timeoutAction: "none",
        electors,
      },
      timeoutSeconds: 86400,
    },
    {
      key: "record",
      name: "结果归档",
      type: "task",
      ...actor(2),
      dependsOn: ["decision"],
    },
  ];
};
export async function createExampleWorkflow(
  kind: Example,
  values: ExampleValues,
  agents: Agent[],
  members: GroupMember[],
) {
  const copy = exampleCopy[kind];
  const steps = buildExampleSteps(
    kind,
    values,
    new Map(agents.map((agent) => [agent.id, agent.name])),
    new Map(members.map((member) => [member.agentId, member.role])),
  );
  const created = unwrapResponse<{ id: string }>(
    await workflowApi.create({
      name: copy.name,
      description: copy.description,
    }),
  );
  const version = unwrapResponse<{ id: string }>(
    await workflowApi.createVersion(created.id, {
      steps,
      inputSchema: { type: "object" },
      outputSchema: { type: "object" },
    }),
  );
  await workflowApi.publishVersion(version.id);
  const execution = values.startNow
    ? unwrapResponse<WorkflowExecution>(
        await workflowApi.start(version.id, {
          input: {},
          idempotencyKey: crypto.randomUUID(),
        }),
      )
    : undefined;
  return { workflowId: created.id, versionId: version.id, execution };
}
const statusText: Record<string, string> = {
  draft: "草稿",
  published: "可使用",
  pending: "待处理",
  running: "进行中",
  waiting: "等待处理",
  waiting_approval: "等待批准",
  waiting_decision: "等待表决",
  approved: "已批准",
  skipped: "已跳过",
  completed: "已完成",
  failed: "失败",
  timed_out: "已超时",
  open: "投票中",
  passed: "已通过",
  rejected: "未通过",
  no_quorum: "人数不足",
  escalated: "已升级",
};

export default function GovernancePage() {
  const { styles, cx } = useStyles();
  const primary = usePrimaryButtonStyle();
  const canWrite = useCanWrite();
  const workflows = useWorkflows();
  const decisions = useDecisions();
  const groups = useGroups();
  const agents = useAgents();
  const [workflowId, setWorkflowId] = useState<string>();
  const [executionId, setExecutionId] = useState<string>();
  const [decisionId, setDecisionId] = useState<string>();
  const [example, setExample] = useState<Example>();
  const [exampleOpen, setExampleOpen] = useState(false);
  const [exampleGroupId, setExampleGroupId] = useState<string>();
  const [decisionOpen, setDecisionOpen] = useState(false);
  const [voteOpen, setVoteOpen] = useState(false);
  const [exampleForm] = Form.useForm<ExampleValues>();
  const [decisionForm] = Form.useForm();
  const [voteForm] = Form.useForm();
  const selectedWorkflowId = workflowId ?? workflows.data?.[0]?.id;
  const workflow = useWorkflow(selectedWorkflowId);
  const execution = useWorkflowExecution(executionId);
  const executions = useWorkflowExecutions(selectedWorkflowId);
  const selectedDecisionId = decisionId ?? decisions.data?.[0]?.id;
  const decision = useDecision(selectedDecisionId);
  const decisionAudit = useDecisionAudit(selectedDecisionId);
  const [groupId, setGroupId] = useState<string>();
  const members = useGroupMembers(groupId);
  const exampleMembers = useGroupMembers(exampleGroupId);
  const agentOptions = useMemo(
    () =>
      (agents.data ?? []).map((a) => ({
        value: a.id,
        label: a.config.title?.["zh-CN"] || a.name,
      })),
    [agents.data],
  );
  const agentName = (id: number) =>
    agentOptions.find((a) => a.value === id)?.label ?? `Agent #${id}`;
  const makeExample = useGovernanceAction(
    async (input: { kind: Example; values: ExampleValues }) => {
      const result = await createExampleWorkflow(
        input.kind,
        input.values,
        agents.data ?? [],
        exampleMembers.data ?? [],
      );
      setWorkflowId(result.workflowId);
      if (result.execution) setExecutionId(result.execution.id);
    },
    "案例已创建，可以直接查看每一步",
  );
  const createDecision = useGovernanceAction(
    (data: Parameters<typeof decisionApi.create>[0]) =>
      decisionApi.create(data),
    "表决已发起，成员与权重已经冻结",
  );
  const castVote = useGovernanceAction(
    (input: {
      id: string;
      agentId: number;
      choice: DecisionChoice;
      reason?: string;
    }) => decisionApi.vote(input.id, input),
    "投票已记录",
  );
  const closeDecision = useGovernanceAction(
    (id: string) => decisionApi.close(id),
    "表决结果已经形成",
  );
  const startWorkflow = useGovernanceAction(async (versionId: string) => {
    const res = unwrapResponse<WorkflowExecution>(
      await workflowApi.start(versionId, {
        input: {},
        idempotencyKey: crypto.randomUUID(),
      }),
    );
    setExecutionId(res.id);
  }, "流程已开始");
  const selectedVersion =
    workflow.data?.versions?.find((v) => v.status === "published") ??
    workflow.data?.versions?.at(-1);
  const submitDecision = async () => {
    const v = (await decisionForm.validateFields()) as {
      groupId: string;
      title: string;
      description?: string;
      quorumPercent: number;
      approvalPercent: number;
      timeoutAction: "none" | "escalate" | "transfer";
      escalateAgentId?: number;
      deadlineAt?: string;
    };
    const electors = (members.data ?? []).map((m) => ({
      agentId: m.agentId,
      weight: 1,
      canVeto: m.role === "leader",
    }));
    await createDecision.mutateAsync({ ...v, electors });
    decisionForm.resetFields();
    setDecisionOpen(false);
  };
  return (
    <div className={styles.page}>
      <div className={styles.head}>
        <div>
          <div className={styles.title}>流程与决策</div>
          <div className={styles.sub}>
            看清工作走到哪一步、现在等谁处理，以及一次表决为什么通过或失败。每个动作都能按时间重放。
          </div>
        </div>
      </div>
      <Tabs
        items={[
          {
            key: "workflow",
            label: "工作流",
            children: (
              <>
                <Alert
                  showIcon
                  type="info"
                  title="工作流把重复的协作方式保存下来"
                  description="串行、并行、交接、会签与超时升级都由步骤定义；业务名称只是模板内容，不改变平台能力。"
                />
                <div className={styles.examples}>
                  <span className={styles.muted}>快速建立验收案例：</span>
                  {(Object.keys(exampleCopy) as Example[]).map((k) => (
                    <Button
                      key={k}
                      onClick={() => {
                        setExample(k);
                        setExampleGroupId(undefined);
                        exampleForm.resetFields();
                        setExampleOpen(true);
                      }}
                    >
                      {exampleCopy[k].label}
                    </Button>
                  ))}
                </div>
                <div className={styles.ledger}>
                  <aside className={styles.rail}>
                    <div className={styles.section}>流程模板</div>
                    {workflows.isLoading ? (
                      <Spin />
                    ) : (
                      <div className={styles.list}>
                        {workflows.data?.map((w) => (
                          <button
                            key={w.id}
                            type="button"
                            className={cx(
                              styles.item,
                              w.id === selectedWorkflowId && styles.active,
                            )}
                            onClick={() => {
                              setWorkflowId(w.id);
                              setExecutionId(undefined);
                            }}
                          >
                            <div className={styles.name}>{w.name}</div>
                            <div className={styles.muted}>
                              {w.description || "暂无说明"}
                            </div>
                          </button>
                        ))}
                      </div>
                    )}
                    <div className={styles.section}>执行档案</div>
                    <div className={styles.list}>
                      {executions.data?.map((e) => (
                        <button
                          key={e.id}
                          type="button"
                          className={cx(
                            styles.item,
                            e.id === executionId && styles.active,
                          )}
                          onClick={() => setExecutionId(e.id)}
                        >
                          <div className={styles.row}>
                            <span className={styles.name}>
                              执行 {e.id.slice(0, 8)}
                            </span>
                            <Tag>{statusText[e.status] ?? e.status}</Tag>
                          </div>
                          <div className={styles.muted}>
                            {e.startedAt
                              ? new Date(e.startedAt).toLocaleString()
                              : "等待开始"}
                          </div>
                        </button>
                      ))}
                    </div>
                  </aside>
                  <main className={styles.detail}>
                    {!workflow.data ? (
                      <div className={styles.empty}>
                        <Empty description="先建立一个流程模板" />
                      </div>
                    ) : (
                      <>
                        <div className={styles.row}>
                          <div>
                            <h2>{workflow.data.name}</h2>
                            <div className={styles.muted}>
                              {workflow.data.description}
                            </div>
                          </div>
                          {selectedVersion && (
                            <Tag>
                              {statusText[selectedVersion.status] ??
                                selectedVersion.status}
                            </Tag>
                          )}
                        </div>
                        <div className={styles.section}>如何推进</div>
                        {selectedVersion?.steps.map((s, i) => (
                          <div className={styles.stageLine} key={s.key}>
                            <span className={styles.stageDot}>{i + 1}</span>
                            <div>
                              <div className={styles.name}>{s.name}</div>
                              <div className={styles.muted}>
                                {s.dependsOn?.length
                                  ? `等待：${s.dependsOn.join("、")}`
                                  : "流程起点"}{" "}
                                ·{" "}
                                {s.type === "approval"
                                  ? "需要形成决定"
                                  : s.type === "handoff"
                                    ? "交给下一位"
                                    : "执行任务"}
                              </div>
                            </div>
                            {s.timeoutSeconds && (
                              <Tag icon={<WarningIcon />}>
                                {Math.round(s.timeoutSeconds / 3600)}{" "}
                                小时未处理则升级
                              </Tag>
                            )}
                          </div>
                        ))}
                        {canWrite &&
                          selectedVersion?.status === "published" && (
                            <PrimaryButton
                              style={{ marginTop: 16 }}
                              onClick={() =>
                                startWorkflow.mutate(selectedVersion.id)
                              }
                            >
                              开始一次流程
                            </PrimaryButton>
                          )}
                        {execution.data && (
                          <ExecutionPanel
                            execution={execution.data}
                            styles={styles}
                          />
                        )}
                      </>
                    )}
                  </main>
                </div>
              </>
            ),
          },
          {
            key: "decision",
            label: "投票与决定",
            children: (
              <>
                <div className={styles.row}>
                  <Alert
                    style={{ flex: 1 }}
                    showIcon
                    type="info"
                    title="投票开始后，参与者、角色和权重会被冻结"
                    description="后续成员变化不会改写本次结果；人数门槛、通过门槛、否决和超时处理都会展示在结果说明中。"
                  />
                  {canWrite && (
                    <PrimaryButton
                      icon={<PlusIcon />}
                      onClick={() => setDecisionOpen(true)}
                    >
                      发起表决
                    </PrimaryButton>
                  )}
                </div>
                <div className={styles.ledger} style={{ marginTop: 18 }}>
                  <aside className={styles.rail}>
                    <div className={styles.section}>全部表决</div>
                    <div className={styles.list}>
                      {decisions.data?.map((d) => (
                        <button
                          key={d.id}
                          type="button"
                          className={cx(
                            styles.item,
                            d.id === selectedDecisionId && styles.active,
                          )}
                          onClick={() => setDecisionId(d.id)}
                        >
                          <div className={styles.row}>
                            <span className={styles.name}>{d.title}</span>
                            <Tag>{statusText[d.status] ?? d.status}</Tag>
                          </div>
                          <div className={styles.muted}>
                            {d.description || "暂无说明"}
                          </div>
                        </button>
                      ))}
                    </div>
                  </aside>
                  <main className={styles.detail}>
                    {decision.data ? (
                      <DecisionPanel
                        value={decision.data}
                        audit={decisionAudit.data ?? []}
                        agentName={agentName}
                        canWrite={canWrite}
                        styles={styles}
                        onVote={() => setVoteOpen(true)}
                        onClose={() => closeDecision.mutate(decision.data!.id)}
                      />
                    ) : (
                      <div className={styles.empty}>
                        <Empty description="还没有表决，发起后可在这里查看依据" />
                      </div>
                    )}
                  </main>
                </div>
              </>
            ),
          },
        ]}
      />
      <ExampleWizard
        open={exampleOpen}
        example={example}
        groups={groups.data ?? []}
        members={exampleMembers.data ?? []}
        agents={agents.data ?? []}
        loading={makeExample.isPending}
        form={exampleForm}
        primaryClassName={primary.root}
        onGroupChange={setExampleGroupId}
        onCancel={() => setExampleOpen(false)}
        onSubmit={async (values) => {
          if (!exampleMembers.data?.length) {
            message.warning("这个群组还没有成员，请先到“群组与频道”中添加成员");
            return;
          }
          const validIds = values.agentIds.filter(
            (id) =>
              exampleMembers.data?.some((member) => member.agentId === id) &&
              agents.data?.some((agent) => agent.id === id),
          );
          if (!validIds.length) {
            message.warning("请选择群组中的实际 Agent，才能把任务交给他");
            return;
          }
          if (
            example === "content" &&
            !exampleMembers.data.some(
              (member) => member.role === values.approvalRole,
            )
          ) {
            message.warning("请选择这个群组中实际存在的确认角色");
            return;
          }
          await makeExample.mutateAsync({
            kind: example as Example,
            values: { ...values, agentIds: validIds },
          });
          setExampleOpen(false);
        }}
      />
      <Modal
        title="发起表决"
        open={decisionOpen}
        onCancel={() => setDecisionOpen(false)}
        onOk={() => void submitDecision()}
        okText="发起并冻结名单"
        cancelText="取消"
        okButtonProps={{ className: primary.root }}
      >
        <Form
          form={decisionForm}
          layout="vertical"
          initialValues={{
            quorumPercent: 60,
            approvalPercent: 50,
            timeoutAction: "escalate",
          }}
        >
          <Form.Item
            name="groupId"
            label="由哪个群组表决"
            rules={[{ required: true, message: "请选择群组" }]}
          >
            <Select
              options={(groups.data ?? []).map((g) => ({
                value: g.id,
                label: g.name,
              }))}
              onChange={setGroupId}
            />
          </Form.Item>
          <Form.Item
            name="title"
            label="要决定什么"
            rules={[{ required: true, message: "请输入表决标题" }]}
          >
            <Input placeholder="例如：是否发布本期研究报告" />
          </Form.Item>
          <Form.Item name="description" label="判断背景">
            <Input.TextArea />
          </Form.Item>
          <div className={styles.row}>
            <Form.Item name="quorumPercent" label="至少多少权重参与">
              <InputNumber min={1} max={100} addonAfter="%" />
            </Form.Item>
            <Form.Item name="approvalPercent" label="参与票中多少算通过">
              <InputNumber min={1} max={100} addonAfter="%" />
            </Form.Item>
          </div>
          <Form.Item name="timeoutAction" label="超时怎么办">
            <Select
              options={[
                { value: "none", label: "只标记超时" },
                { value: "escalate", label: "升级给指定 Agent" },
                { value: "transfer", label: "转交处理" },
              ]}
            />
          </Form.Item>
          <Form.Item name="escalateAgentId" label="升级给谁（可选）">
            <Select allowClear options={agentOptions} />
          </Form.Item>
          <div className={styles.muted}>
            当前将冻结 {members.data?.length ?? 0}{" "}
            名群组成员；负责人默认拥有否决权。
          </div>
        </Form>
      </Modal>
      <Modal
        title="记录投票"
        open={voteOpen}
        onCancel={() => setVoteOpen(false)}
        onOk={() =>
          void voteForm.validateFields().then((v) =>
            castVote
              .mutateAsync({ id: selectedDecisionId as string, ...v })
              .then(() => {
                voteForm.resetFields();
                setVoteOpen(false);
              }),
          )
        }
        okButtonProps={{ className: primary.root }}
        okText="记录投票"
      >
        <Form form={voteForm} layout="vertical">
          <Form.Item name="agentId" label="投票人" rules={[{ required: true }]}>
            <Select
              options={decision.data?.electorate?.map((e) => ({
                value: e.agentId,
                label: agentName(e.agentId),
              }))}
            />
          </Form.Item>
          <Form.Item name="choice" label="选择" rules={[{ required: true }]}>
            <Select
              options={[
                { value: "approve", label: "赞成" },
                { value: "reject", label: "反对" },
                { value: "abstain", label: "弃权" },
              ]}
            />
          </Form.Item>
          <Form.Item name="reason" label="理由">
            <Input.TextArea />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

const roleLabels: Record<GroupMemberRole, string> = {
  leader: "负责人",
  member: "成员",
  observer: "观察者",
  guest: "访客",
};

function ExampleWizard({
  open,
  example,
  groups,
  members,
  agents,
  loading,
  form,
  primaryClassName,
  onGroupChange,
  onCancel,
  onSubmit,
}: {
  open: boolean;
  example?: Example;
  groups: CollaborationGroup[];
  members: GroupMember[];
  agents: Agent[];
  loading: boolean;
  form: ReturnType<typeof Form.useForm<ExampleValues>>[0];
  primaryClassName: string;
  onGroupChange: (id: string) => void;
  onCancel: () => void;
  onSubmit: (values: ExampleValues) => Promise<void>;
}) {
  const hasGroups = groups.length > 0;
  const actualMembers = members.filter((member) =>
    agents.some((agent) => agent.id === member.agentId),
  );
  const roles = Array.from(new Set(members.map((member) => member.role)));
  const label = (id: number) => {
    const agent = agents.find((item) => item.id === id);
    return agent?.config.title?.["zh-CN"] || agent?.name || `Agent #${id}`;
  };
  return (
    <Modal
      title={example ? `建立「${exampleCopy[example].label}」案例` : "建立案例"}
      open={open}
      onCancel={onCancel}
      okText="创建并运行"
      cancelText="取消"
      confirmLoading={loading}
      okButtonProps={{ className: primaryClassName, disabled: !hasGroups }}
      onOk={() => void form.validateFields().then(onSubmit)}
    >
      {!hasGroups ? (
        <Alert
          showIcon
          type="warning"
          title="还不能建立案例"
          description="请先到“群组与频道”建立一个群组并添加成员，再回来选择真实参与者。"
        />
      ) : (
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            agentIds: [],
            approvalRole: "leader",
            startNow: true,
          }}
        >
          <Form.Item
            name="groupId"
            label="使用哪个群组"
            rules={[{ required: true, message: "请选择一个现有群组" }]}
          >
            <Select
              placeholder="选择群组"
              options={groups.map((group) => ({
                value: group.id,
                label: group.name,
              }))}
              onChange={(id: string) => {
                onGroupChange(id);
                form.setFieldsValue({ agentIds: [] });
              }}
            />
          </Form.Item>
          {form.getFieldValue("groupId") && members.length === 0 && (
            <Alert
              style={{ marginBottom: 16 }}
              showIcon
              type="warning"
              title="这个群组还没有成员"
              description="先为群组添加 Agent，案例才能真正运行。"
            />
          )}
          <Form.Item
            name="agentIds"
            label="由哪些 Agent 执行"
            rules={[
              {
                required: true,
                type: "array",
                min: 1,
                message: "至少选择一位群组成员",
              },
            ]}
          >
            <Select
              mode="multiple"
              placeholder="选择实际参与者"
              options={actualMembers.map((member) => ({
                value: member.agentId,
                label: `${label(member.agentId)} · ${roleLabels[member.role]}`,
              }))}
            />
          </Form.Item>
          {example === "content" && (
            <Form.Item
              name="approvalRole"
              label="最后由哪类成员确认"
              rules={[{ required: true, message: "请选择确认角色" }]}
            >
              <Select
                placeholder="选择群组内已有角色"
                options={roles.map((role) => ({
                  value: role,
                  label: roleLabels[role],
                }))}
              />
            </Form.Item>
          )}
          <Form.Item name="startNow" valuePropName="checked">
            <Checkbox>创建后立即开始，方便直接验收</Checkbox>
          </Form.Item>
          <div style={{ color: t.textMuted, fontSize: t.textSm }}>
            {example === "asset"
              ? "表决名单会按你选中的群组成员冻结；负责人默认拥有否决权。"
              : "系统会使用所选 Agent 的真实名称派发任务，并按群组角色寻找最终确认人。"}
          </div>
        </Form>
      )}
    </Modal>
  );
}

function ExecutionPanel({
  execution,
  styles,
}: {
  execution: WorkflowExecution;
  styles: ReturnType<typeof useStyles>["styles"];
}) {
  return (
    <>
      <div className={styles.section}>这次执行到哪里</div>
      <Alert
        showIcon
        type={execution.status === "failed" ? "error" : "success"}
        title={statusText[execution.status] ?? execution.status}
        description={
          execution.currentStepKey
            ? `正在等待步骤：${execution.currentStepKey}`
            : "状态已经写入执行档案"
        }
      />
      {execution.stepRuns?.map((s, i) => (
        <div className={styles.stageLine} key={s.id}>
          <span className={styles.stageDot}>
            {s.status === "completed" ? <CheckCircleIcon /> : i + 1}
          </span>
          <div>
            <div className={styles.name}>{s.name || s.stepKey}</div>
            <div className={styles.muted}>
              {s.error || `第 ${s.attempt ?? 1} 次处理`}
            </div>
          </div>
          <Tag>{statusText[s.status] ?? s.status}</Tag>
        </div>
      ))}
      {execution.approvals?.length ? (
        <>
          <div className={styles.section}>审批记录</div>
          {execution.approvals.map((a) => (
            <div className={styles.stageLine} key={a.id}>
              <span className={styles.stageDot}>
                {a.decisions?.length ?? 0}
              </span>
              <div>
                <div className={styles.name}>
                  {a.policy === "all" ? "需要全部同意" : "达到人数门槛即可"}
                  （至少 {a.quorum ?? 1} 人）
                </div>
                <div className={styles.muted}>
                  {a.decisions?.length
                    ? a.decisions
                        .map(
                          (d) =>
                            `${d.actorId}：${d.decision === "approve" ? "同意" : d.decision === "reject" ? "拒绝" : "有条件同意"}${d.reason ? `，${d.reason}` : ""}`,
                        )
                        .join("；")
                    : "正在等待审批人处理"}
                </div>
              </div>
              <Tag>{statusText[a.status] ?? a.status}</Tag>
            </div>
          ))}
        </>
      ) : null}
    </>
  );
}
function DecisionPanel({
  value,
  audit,
  agentName,
  canWrite,
  styles,
  onVote,
  onClose,
}: {
  value: CollectiveDecision;
  audit: Array<{
    id: string;
    action: string;
    description?: string;
    createdAt: string;
  }>;
  agentName: (id: number) => string;
  canWrite: boolean;
  styles: ReturnType<typeof useStyles>["styles"];
  onVote: () => void;
  onClose: () => void;
}) {
  const total = value.electorate?.reduce((n, e) => n + e.weight, 0) ?? 0;
  const voted =
    value.votes?.reduce(
      (n, v) =>
        n +
        (v.weight ??
          value.electorate?.find((e) => e.agentId === v.agentId)?.weight ??
          0),
      0,
    ) ?? 0;
  return (
    <>
      <div className={styles.row}>
        <div>
          <h2>{value.title}</h2>
          <div className={styles.muted}>{value.description}</div>
        </div>
        <Tag>{statusText[value.status] ?? value.status}</Tag>
      </div>
      <div className={styles.stats}>
        <div className={styles.stat}>
          <div className="value">{value.quorumPercent}%</div>
          <div className={styles.muted}>参与门槛</div>
        </div>
        <div className={styles.stat}>
          <div className="value">{value.approvalPercent}%</div>
          <div className={styles.muted}>通过门槛</div>
        </div>
        <div className={styles.stat}>
          <div className="value">{value.electorate?.length ?? 0}</div>
          <div className={styles.muted}>名单已冻结</div>
        </div>
      </div>
      <Progress
        percent={total ? Math.round((voted / total) * 100) : 0}
        format={(p) => `已投 ${p}% 权重`}
      />
      <div className={styles.section}>谁投了什么</div>
      {value.electorate?.map((e) => {
        const vote = value.votes?.find((v) => v.agentId === e.agentId);
        return (
          <div className={styles.stageLine} key={e.agentId}>
            <span className={styles.stageDot}>{e.weight}</span>
            <div>
              <div className={styles.name}>
                {agentName(e.agentId)}{" "}
                {e.canVeto && <Tag color="red">有否决权</Tag>}
              </div>
              <div className={styles.muted}>
                {vote?.reason || "尚未投票"} · 冻结角色：{e.role || "成员"}
              </div>
            </div>
            <Tag>
              {vote
                ? { approve: "赞成", reject: "反对", abstain: "弃权" }[
                    vote.choice
                  ]
                : "待处理"}
            </Tag>
          </div>
        );
      })}
      {value.result && (
        <Alert
          style={{ marginTop: 14 }}
          showIcon
          type={value.status === "passed" ? "success" : "warning"}
          title={value.result.explanation}
          description={`参与门槛${value.result.quorumMet ? "已满足" : "未满足"}；${value.result.vetoed ? "存在否决票" : "没有触发否决"}`}
        />
      )}{" "}
      {canWrite && value.status === "open" && (
        <div className={styles.actions}>
          <PrimaryButton onClick={onVote}>记录一票</PrimaryButton>
          <Button onClick={onClose}>结束并计算结果</Button>
        </div>
      )}
      <div className={styles.section}>形成过程</div>
      {audit.length ? (
        <Timeline
          items={audit.map((a) => ({
            children: (
              <>
                <strong>{a.description || a.action}</strong>
                <div className={styles.muted}>
                  {new Date(a.createdAt).toLocaleString()}
                </div>
              </>
            ),
          }))}
        />
      ) : (
        <Empty description="暂无变更记录" />
      )}
    </>
  );
}
