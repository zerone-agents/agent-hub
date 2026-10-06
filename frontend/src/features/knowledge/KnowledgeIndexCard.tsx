import { useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Empty,
  Popconfirm,
  Progress,
  Space,
  Spin,
  Tag,
  Typography,
} from "antd";
import {
  useKnowledgeAction,
  useKnowledgeResource,
} from "@/queries/useKnowledgeManagement";
import { parseApiError } from "@/api/client";
import type { KnowledgeObject } from "@/api/knowledgeManagement";
import PrimaryButton from "@/components/PrimaryButton";
import { usePrimaryButtonStyle } from "@/components/PrimaryButton";
import { useCanWrite } from "@/hooks/useCanWrite";
import {
  cancellationMarkerCount,
  getIndexState,
  knowledgeObject,
} from "./knowledgeDiagnostics";
import {
  KnowledgeDiagnosticDetails,
  KnowledgeRawData,
} from "./KnowledgeDiagnosticDetails";
import { useDiagnosticsText } from "./knowledgeDiagnostics.messages";

export default function KnowledgeIndexCard({
  id,
  kind,
}: {
  id: string;
  kind: "graph" | "raptor" | "mindmap";
}) {
  const text = useDiagnosticsText();
  const canWrite = useCanWrite();
  const primary = usePrimaryButtonStyle();
  const task = useKnowledgeResource<KnowledgeObject>(
    id,
    "index",
    { type: kind },
    true,
  );
  const action = useKnowledgeAction(id);
  const [feedback, setFeedback] = useState<{
    message: string;
    error: boolean;
    activeTaskId?: string;
  }>();
  const busy = useRef(false);
  const confirmationTaskId = useRef<unknown>(undefined);
  const [operating, setOperating] = useState(false);
  const [cancelBaseline, setCancelBaseline] = useState<{
    taskId: unknown;
    markerCount: number;
  }>();
  const state = getIndexState(task.data, cancelBaseline);
  const active = state === "queued" || state === "running";
  const unavailable =
    Boolean(task.error) || task.isLoading || !task.data || state === "unknown";
  const title =
    kind === "graph" ? "GraphRAG" : kind === "raptor" ? "RAPTOR" : "Mindmap";
  const progress =
    typeof task.data?.progress === "number" &&
    Number.isFinite(task.data.progress) &&
    task.data.progress >= 0
      ? Math.max(0, Math.min(100, Math.round(task.data.progress * 100)))
      : undefined;
  const labels = {
    empty: "noIndex",
    unknown: "unknown",
    cancelRequested: "cancelRequestedStatus",
    endedUnconfirmed: "endedUnconfirmed",
    queued: "queued",
    running: "running",
    done: "done",
    failed: "failed",
  } as const;

  async function perform(operation: "run" | "cancel" | "clear") {
    if (busy.current) return;
    busy.current = true;
    setOperating(true);
    setFeedback(undefined);
    try {
      // Re-read before acting: an old confirmation must not cancel a replacement task.
      const expectedId =
        operation === "run" ? task.data?.id : confirmationTaskId.current;
      const latest = await task.refetch();
      const current = getIndexState(latest.data);
      if (
        latest.error ||
        !latest.data ||
        current === "unknown" ||
        (operation !== "run" && latest.data.id !== expectedId) ||
        (operation === "cancel"
          ? current !== "running" && current !== "queued"
          : current === "running" || current === "queued")
      ) {
        setFeedback({ message: text("taskUnavailable"), error: true });
        return;
      }
      if (operation === "cancel") {
        setCancelBaseline({
          taskId: expectedId,
          markerCount: cancellationMarkerCount(latest.data.progress_msg),
        });
      } else if (operation === "run") {
        setCancelBaseline(undefined);
      }
      const accepted = await action.mutateAsync({
        method: operation === "clear" ? "delete" : "post",
        resource: operation === "cancel" ? "index/cancel" : "index",
        params: { type: kind },
        ...(operation === "cancel" ? { data: { task_id: expectedId } } : {}),
      });
      const readback = await task.refetch();
      const response = knowledgeObject(accepted);
      const acceptedId = response?.task_id;
      if (
        operation === "cancel" &&
        (response?.request_accepted !== true ||
          typeof response.cancel_requested !== "boolean" ||
          acceptedId !== expectedId)
      ) {
        setFeedback({ message: text("cancelUnconfirmed"), error: true });
        return;
      }
      const confirmed =
        !readback.error &&
        readback.data &&
        (operation === "clear"
          ? Object.keys(readback.data).length === 0
          : operation === "cancel"
            ? readback.data.id === expectedId &&
              getIndexState(readback.data) !== "unknown" &&
              getIndexState(readback.data) !== "empty"
            : typeof acceptedId === "string" &&
              readback.data.id === acceptedId);
      setFeedback(
        !confirmed
          ? { message: text("readbackFailed"), error: true }
          : operation === "cancel" &&
              typeof acceptedId === "string" &&
              (getIndexState(readback.data) === "running" ||
                getIndexState(readback.data) === "queued")
            ? {
                message: text("cancelPending"),
                error: false,
                activeTaskId: acceptedId,
              }
            : undefined,
      );
    } catch (error) {
      setFeedback({ message: parseApiError(error), error: true });
    } finally {
      busy.current = false;
      setOperating(false);
    }
  }

  return (
    <Card size="small" title={title} style={{ minWidth: 0 }}>
      <Space orientation="vertical" size="middle" style={{ width: "100%" }}>
        <Typography.Text type="secondary">
          {text(
            kind === "graph"
              ? "graphHint"
              : kind === "raptor"
                ? "raptorHint"
                : "mindmapHint",
          )}
        </Typography.Text>
        {task.error ? (
          <Alert type="error" showIcon title={parseApiError(task.error)} />
        ) : task.isLoading ? (
          <Spin />
        ) : state === "empty" ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={text("noIndex")}
          />
        ) : (
          <>
            <Tag
              color={
                state === "done"
                  ? "success"
                  : state === "failed"
                    ? "error"
                    : active
                      ? "processing"
                      : "default"
              }
            >
              {text(labels[state])}
            </Tag>
            {progress !== undefined && (
              <Progress
                percent={progress}
                status={
                  state === "failed"
                    ? "exception"
                    : state === "done"
                      ? "success"
                      : active
                        ? "active"
                        : "normal"
                }
              />
            )}
            {task.data && (
              <>
                <KnowledgeDiagnosticDetails
                  data={{ ...task.data, task_id: task.data.id }}
                />
                <KnowledgeRawData data={task.data} />
              </>
            )}
          </>
        )}
        {state === "cancelRequested" && (
          <Alert type="info" showIcon title={text("cancelRequested")} />
        )}
        {state === "endedUnconfirmed" && (
          <Alert type="info" showIcon title={text("cancellationHistoryHint")} />
        )}
        {feedback &&
          (!feedback.activeTaskId ||
            (active && task.data?.id === feedback.activeTaskId)) && (
            <Alert
              type={feedback.error ? "error" : "info"}
              showIcon
              title={feedback.message}
            />
          )}
        {active && (
          <Typography.Text type="secondary">
            {text("activeHint")}
          </Typography.Text>
        )}
        <Space wrap>
          <Button
            loading={task.isFetching}
            onClick={() => {
              setFeedback(undefined);
              void task.refetch();
            }}
          >
            {text("refresh")}
          </Button>
          {canWrite && (
            <>
              <PrimaryButton
                disabled={
                  unavailable || active || operating || action.isPending
                }
                loading={operating || action.isPending}
                onClick={() => void perform("run")}
              >
                {text("run")}
              </PrimaryButton>
              <Popconfirm
                onOpenChange={(open) => {
                  if (open) confirmationTaskId.current = task.data?.id;
                }}
                title={text("cancelTitle", { kind: title })}
                description={
                  <div style={{ maxWidth: 300 }}>{text("cancelHint")}</div>
                }
                okText={text("cancel")}
                okButtonProps={{ className: primary.root }}
                disabled={
                  unavailable || !active || operating || action.isPending
                }
                onConfirm={() => perform("cancel")}
              >
                <Button
                  disabled={
                    unavailable || !active || operating || action.isPending
                  }
                >
                  {text("cancel")}
                </Button>
              </Popconfirm>
              <Popconfirm
                onOpenChange={(open) => {
                  if (open) confirmationTaskId.current = task.data?.id;
                }}
                title={text("clearTitle", { kind: title })}
                description={
                  <div style={{ maxWidth: 300 }}>
                    {text(
                      kind === "mindmap" ? "mindmapClearHint" : "clearHint",
                    )}
                  </div>
                }
                okText={text(kind === "mindmap" ? "clearMindmap" : "clear")}
                okButtonProps={{ className: primary.root, danger: true }}
                disabled={
                  unavailable || active || operating || action.isPending
                }
                onConfirm={() => perform("clear")}
              >
                <Button
                  danger
                  disabled={
                    unavailable || active || operating || action.isPending
                  }
                >
                  {text(kind === "mindmap" ? "clearMindmap" : "clear")}
                </Button>
              </Popconfirm>
            </>
          )}
        </Space>
      </Space>
    </Card>
  );
}
