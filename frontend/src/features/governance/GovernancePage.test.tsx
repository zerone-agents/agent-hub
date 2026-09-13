import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import GovernancePage, {
  buildExampleSteps,
  createExampleWorkflow,
} from "./GovernancePage";
import { workflowApi } from "@/api/workflows";
const state = vi.hoisted(() => ({
  groups: [{ id: "g1", name: "内容团队" }] as Array<{
    id: string;
    name: string;
  }>,
  members: [{ agentId: 1, role: "leader" as const }],
}));
vi.mock("@/hooks/useCanWrite", () => ({ useCanWrite: () => true }));
vi.mock("@/queries/useAgents", () => ({
  useAgents: () => ({
    data: [
      { id: 1, name: "legal", config: { title: { "zh-CN": "法务 Agent" } } },
    ],
  }),
}));
vi.mock("@/queries/useGroups", () => ({
  useGroups: () => ({ data: state.groups }),
  useGroupMembers: () => ({ data: state.members }),
}));
vi.mock("@/queries/useGovernance", () => ({
  useWorkflows: () => ({
    data: [
      { id: "w1", name: "内容发布标准流程", description: "发布前完成审核" },
    ],
  }),
  useWorkflow: () => ({
    data: {
      id: "w1",
      name: "内容发布标准流程",
      versions: [
        {
          id: "v1",
          status: "published",
          steps: [
            {
              key: "facts",
              name: "事实审核",
              type: "task",
              actorType: "role",
              actorRef: "checker",
            },
            {
              key: "approve",
              name: "负责人批准",
              type: "approval",
              actorType: "role",
              actorRef: "owner",
              dependsOn: ["facts"],
              timeoutSeconds: 3600,
            },
          ],
        },
      ],
    },
  }),
  useWorkflowExecution: () => ({ data: undefined }),
  useWorkflowExecutions: () => ({ data: [] }),
  useDecisions: () => ({
    data: [{ id: "d1", title: "是否发布报告", status: "open" }],
  }),
  useDecision: () => ({
    data: {
      id: "d1",
      groupId: "g1",
      title: "是否发布报告",
      description: "核验事实后发布",
      quorumPercent: 60,
      approvalPercent: 50,
      timeoutAction: "escalate",
      status: "open",
      createdAt: "2026-01-01",
      electorate: [{ agentId: 1, weight: 1, canVeto: true, role: "leader" }],
      votes: [],
    },
  }),
  useDecisionAudit: () => ({ data: [] }),
  useGovernanceAction: () => ({
    mutate: vi.fn(),
    mutateAsync: vi.fn(),
    isPending: false,
  }),
}));
describe("GovernancePage", () => {
  beforeEach(() => {
    state.groups = [{ id: "g1", name: "内容团队" }];
    state.members = [{ agentId: 1, role: "leader" }];
    vi.restoreAllMocks();
  });
  it("解释工作如何推进以及决定如何形成", () => {
    render(<GovernancePage />);
    expect(screen.getAllByText("内容发布标准流程").length).toBeGreaterThan(0);
    expect(screen.getByText("事实审核")).toBeInTheDocument();
    expect(screen.getByText(/1 小时未处理则升级/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "投票与决定" }));
    expect(screen.getByText("谁投了什么")).toBeInTheDocument();
    expect(screen.getByText("法务 Agent")).toBeInTheDocument();
    expect(screen.getByText("有否决权")).toBeInTheDocument();
    expect(screen.getAllByText(/名单已冻结/).length).toBeGreaterThan(0);
  });
  it("没有实际群组时阻止快速案例创建", () => {
    state.groups = [];
    render(<GovernancePage />);
    fireEvent.click(screen.getByRole("button", { name: "内容发布审核" }));
    expect(screen.getByText("还不能建立案例")).toBeInTheDocument();
    expect(
      screen.getByText(/先到“群组与频道”建立一个群组/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建并运行" })).toBeDisabled();
  });
  it("资产案例使用真实群组、实际 Agent 和冻结成员名单", () => {
    const steps = buildExampleSteps(
      "asset",
      { groupId: "group-real", agentIds: [1], approvalRole: "leader" },
      new Map([[1, "legal"]]),
      new Map([
        [1, "leader"],
        [2, "member"],
      ]),
    );
    expect(steps[0]).toMatchObject({ actorType: "agent", actorRef: "legal" });
    expect(steps[2]).toMatchObject({
      type: "decision",
      actorType: "group",
      actorRef: "group-real",
      config: {
        groupId: "group-real",
        electors: [
          { agentId: 1, weight: 1, canVeto: true },
          { agentId: 2, weight: 1, canVeto: false },
        ],
      },
    });
  });
  it("群组没有可执行成员时不会创建半成品流程", async () => {
    const create = vi.spyOn(workflowApi, "create");
    await expect(
      createExampleWorkflow(
        "content",
        { groupId: "g-empty", agentIds: [], approvalRole: "leader" },
        [],
        [],
      ),
    ).rejects.toThrow("请选择至少一位当前群组成员");
    expect(create).not.toHaveBeenCalled();
  });
  it("按真实载荷创建、发布并立即启动案例", async () => {
    vi.spyOn(workflowApi, "create").mockResolvedValue({
      data: { success: true, data: { id: "workflow-real" } },
    } as never);
    const version = vi.spyOn(workflowApi, "createVersion").mockResolvedValue({
      data: { success: true, data: { id: "version-real" } },
    } as never);
    const publish = vi
      .spyOn(workflowApi, "publishVersion")
      .mockResolvedValue({ data: { success: true, data: {} } } as never);
    const start = vi.spyOn(workflowApi, "start").mockResolvedValue({
      data: {
        success: true,
        data: {
          id: "execution-real",
          versionId: "version-real",
          status: "running",
        },
      },
    } as never);
    await createExampleWorkflow(
      "content",
      { groupId: "g1", agentIds: [1], approvalRole: "leader", startNow: true },
      [{ id: 1, name: "legal", config: {} }],
      [{ agentId: 1, role: "leader" }],
    );
    expect(version).toHaveBeenCalledWith(
      "workflow-real",
      expect.objectContaining({
        steps: expect.arrayContaining([
          expect.objectContaining({ actorType: "agent", actorRef: "legal" }),
          expect.objectContaining({
            type: "approval",
            actorType: "role",
            actorRef: "leader",
            config: { groupId: "g1", policy: "all" },
          }),
        ]),
      }),
    );
    expect(publish).toHaveBeenCalledWith("version-real");
    expect(start).toHaveBeenCalledWith(
      "version-real",
      expect.objectContaining({
        input: {},
        idempotencyKey: expect.any(String),
      }),
    );
  });
});
