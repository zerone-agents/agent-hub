import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  Collapse,
  DatePicker,
  Descriptions,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import type { Dayjs } from "dayjs";
import { useKnowledgeResource } from "@/queries/useKnowledgeManagement";
import { parseApiError } from "@/api/client";
import type { KnowledgeObject } from "@/api/knowledgeManagement";
import {
  ingestionLogResource,
  cancellationMarkerCount,
  knowledgeObject,
  knowledgeRows,
  logDocumentId,
  showKnowledgeValue,
} from "./knowledgeDiagnostics";
import {
  KnowledgeDiagnosticDetails,
  KnowledgeRawData,
} from "./KnowledgeDiagnosticDetails";
import KnowledgeIndexCard from "./KnowledgeIndexCard";
import KnowledgeGraphExplorer from "./KnowledgeGraphExplorer";
import { useDiagnosticsText } from "./knowledgeDiagnostics.messages";

const statusKeys = [
  "unstarted",
  "running",
  "cancelled",
  "done",
  "failed",
  "queued",
] as const;

function LogStatus({
  status,
  message,
  progress,
}: {
  status: unknown;
  message?: unknown;
  progress?: unknown;
}) {
  const text = useDiagnosticsText();
  const value =
    typeof status === "string" || typeof status === "number"
      ? String(status)
      : "";
  const known = /^[0-5]$/.test(value);
  const historicalMarker =
    value === "4" && progress === -1 && cancellationMarkerCount(message) > 0;
  return (
    <Space orientation="vertical" size={2}>
      <Tag
        color={
          value === "4"
            ? "error"
            : value === "3"
              ? "success"
              : value === "1" || value === "5"
                ? "processing"
                : "default"
        }
      >
        {known
          ? text(statusKeys[Number(value)])
          : `${text("unknown")}${value ? ` (${value})` : ""}`}
      </Tag>
      {historicalMarker && (
        <Typography.Text
          type="secondary"
          title={text("cancellationHistoryHint")}
          style={{
            fontSize: 12,
            whiteSpace: "normal",
            overflowWrap: "anywhere",
          }}
        >
          {text("cancellationHistory")}
        </Typography.Text>
      )}
    </Space>
  );
}

function KnowledgeLogs({ id }: { id: string }) {
  const text = useDiagnosticsText();
  const [page, setPage] = useState(1);
  const [logType, setLogType] = useState<"file" | "dataset">("file");
  const [status, setStatus] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [keywords, setKeywords] = useState("");
  const [dates, setDates] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [desc, setDesc] = useState(true);
  const [selected, setSelected] = useState<KnowledgeObject>();
  const resource = ingestionLogResource({
    page,
    logType,
    keywords,
    status,
    desc,
    from: dates?.[0]?.toISOString(),
    to: dates?.[1]?.toISOString(),
  });
  const logs = useKnowledgeResource<{ total: number; logs: KnowledgeObject[] }>(
    id,
    resource,
  );
  const logId = typeof selected?.id === "string" ? selected.id : "";
  const detail = useKnowledgeResource<KnowledgeObject>(
    logId ? id : "",
    `ingestions/${encodeURIComponent(logId)}`,
  );
  // File detail currently returns dataset fields; retain the scoped list association.
  const detailData: KnowledgeObject | undefined = detail.data
    ? {
        ...selected,
        ...detail.data,
        document_id: selected?.document_id,
        document_name: selected?.document_name,
        kb_id: selected?.kb_id,
      }
    : undefined;
  const rows = knowledgeRows(logs.data?.logs);

  function documentLink(log: KnowledgeObject) {
    const documentId = logDocumentId(log);
    return documentId && (!log.kb_id || log.kb_id === id) ? (
      <Button
        type="link"
        href={`/knowledge/${encodeURIComponent(id)}/documents/${encodeURIComponent(documentId)}/chunks`}
        style={{ paddingInline: 0 }}
      >
        {text("openDocument")}
      </Button>
    ) : null;
  }

  return (
    <Card
      title={text("logs")}
      size="small"
      extra={
        <Button loading={logs.isFetching} onClick={() => void logs.refetch()}>
          {text("refresh")}
        </Button>
      }
    >
      <Space
        orientation="vertical"
        size="middle"
        style={{ width: "100%", minWidth: 0 }}
      >
        <Typography.Text type="secondary">{text("logsHint")}</Typography.Text>
        <Tabs
          activeKey={logType}
          onChange={(key) => {
            setLogType(key === "file" ? "file" : "dataset");
            setPage(1);
            setSelected(undefined);
          }}
          items={[
            { key: "file", label: text("fileLogs") },
            { key: "dataset", label: text("datasetLogs") },
          ]}
        />
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          <Input.Search
            value={search}
            allowClear
            placeholder={text("keywords")}
            aria-label={text("keywords")}
            onChange={(event) => {
              setSearch(event.target.value);
              if (!event.target.value) {
                setKeywords("");
                setPage(1);
              }
            }}
            onSearch={(value) => {
              setKeywords(value);
              setPage(1);
            }}
            style={{ flex: "1 1 220px", minWidth: 0 }}
          />
          <Select
            mode="multiple"
            allowClear
            value={status}
            aria-label={text("filterStatus")}
            placeholder={text("filterStatus")}
            options={statusKeys.map((key, index) => ({
              label: text(key),
              value: String(index),
            }))}
            onChange={(values) => {
              setStatus(values);
              setPage(1);
            }}
            style={{ flex: "1 1 200px", minWidth: 0 }}
          />
          <DatePicker.RangePicker
            showTime
            value={dates}
            aria-label={text("dateRange")}
            placeholder={[text("dateFrom"), text("dateTo")]}
            onChange={(values) => {
              setDates(values);
              setPage(1);
            }}
            style={{ flex: "1 1 300px", minWidth: 0 }}
          />
          <Select
            value={desc ? "newest" : "oldest"}
            aria-label={text("created")}
            options={[
              { value: "newest", label: text("newest") },
              { value: "oldest", label: text("oldest") },
            ]}
            onChange={(value) => {
              setDesc(value === "newest");
              setPage(1);
            }}
          />
          <Button
            onClick={() => {
              setStatus([]);
              setSearch("");
              setKeywords("");
              setDates(null);
              setDesc(true);
              setPage(1);
            }}
          >
            {text("reset")}
          </Button>
        </div>
        {logs.error ? (
          <Alert type="error" showIcon title={parseApiError(logs.error)} />
        ) : (
          <Table<KnowledgeObject>
            size="small"
            rowKey={(_, index) => String(index)}
            dataSource={rows}
            loading={logs.isFetching}
            scroll={{ x: 760 }}
            locale={{
              emptyText: logs.isLoading ? text("loading") : text("noLogs"),
            }}
            pagination={{
              current: page,
              pageSize: 15,
              total: logs.data?.total ?? 0,
              showSizeChanger: false,
              onChange: setPage,
            }}
            columns={[
              {
                title: text("name"),
                render: (_, row) => (
                  <div style={{ overflowWrap: "anywhere" }}>
                    {showKnowledgeValue(
                      row.document_name ?? row.pipeline_title ?? row.task_type,
                    )}
                    <div>{logType === "file" && documentLink(row)}</div>
                  </div>
                ),
              },
              {
                title: text("status"),
                render: (_, row) => (
                  <LogStatus
                    status={row.operation_status}
                    message={row.progress_msg}
                    progress={row.progress}
                  />
                ),
              },
              {
                title: text("type"),
                render: (_, row) =>
                  showKnowledgeValue(row.task_type ?? row.parser_id),
              },
              {
                title: text("stage"),
                render: (_, row) => (
                  <Typography.Paragraph
                    style={{
                      maxWidth: 320,
                      margin: 0,
                      whiteSpace: "pre-wrap",
                      overflowWrap: "anywhere",
                    }}
                    ellipsis={{ rows: 2, expandable: true }}
                  >
                    {showKnowledgeValue(row.progress_msg)}
                  </Typography.Paragraph>
                ),
              },
              {
                title: text("created"),
                render: (_, row) => showKnowledgeValue(row.create_date),
              },
              {
                title: text("details"),
                render: (_, row) => (
                  <Button
                    type="link"
                    disabled={
                      typeof row.id !== "string" ||
                      !row.id ||
                      Boolean(row.kb_id && row.kb_id !== id)
                    }
                    onClick={() => {
                      setSelected(row);
                    }}
                  >
                    {text("details")}
                  </Button>
                ),
              },
            ]}
          />
        )}
      </Space>
      <Modal
        open={Boolean(selected)}
        title={text("logDetails")}
        onCancel={() => {
          setSelected(undefined);
        }}
        footer={
          <Button
            onClick={() => {
              setSelected(undefined);
            }}
          >
            {text("close")}
          </Button>
        }
      >
        <Space orientation="vertical" style={{ width: "100%" }} size="middle">
          {selected && (
            <>
              <Typography.Text strong style={{ overflowWrap: "anywhere" }}>
                {showKnowledgeValue(
                  selected.document_name ?? selected.task_type,
                )}
              </Typography.Text>
              {documentLink(selected)}
            </>
          )}
          <Button
            loading={detail.isFetching}
            onClick={() => void detail.refetch()}
          >
            {text("refresh")}
          </Button>
          {!logId && selected ? (
            <Alert type="error" title={text("logMissingId")} />
          ) : detail.error ? (
            <Alert type="error" showIcon title={parseApiError(detail.error)} />
          ) : detail.isLoading ? (
            <Spin />
          ) : detailData ? (
            <>
              <LogStatus
                status={detailData.operation_status}
                message={detailData.progress_msg}
                progress={detailData.progress}
              />
              <KnowledgeDiagnosticDetails data={detailData} />
              <KnowledgeRawData data={detailData} />
            </>
          ) : selected ? (
            <Alert type="warning" title={text("noDetails")} />
          ) : null}
        </Space>
      </Modal>
    </Card>
  );
}

function KnowledgeGraph({ id }: { id: string }) {
  const text = useDiagnosticsText();
  const graph = useKnowledgeResource<KnowledgeObject>(id, "graph/search");
  return (
    <Space
      orientation="vertical"
      style={{ width: "100%", minWidth: 0 }}
      size="middle"
    >
      <Button loading={graph.isFetching} onClick={() => void graph.refetch()}>
        {text("refresh")}
      </Button>
      {graph.error ? (
        <Alert type="error" showIcon title={parseApiError(graph.error)} />
      ) : graph.isLoading ? (
        <Spin />
      ) : (
        <KnowledgeGraphExplorer data={graph.data} loading={graph.isFetching} />
      )}
    </Space>
  );
}

function KnowledgeIngestionWorkspace({ id }: { id: string }) {
  const text = useDiagnosticsText();
  const summary = useKnowledgeResource<KnowledgeObject>(
    id,
    "ingestions/summary",
  );
  const status = knowledgeObject(summary.data?.status);
  return (
    <Space
      orientation="vertical"
      style={{ width: "100%", minWidth: 0 }}
      size="middle"
    >
      <Card
        size="small"
        title={text("overview")}
        extra={
          <Button
            loading={summary.isFetching}
            onClick={() => void summary.refetch()}
          >
            {text("refresh")}
          </Button>
        }
      >
        {summary.error ? (
          <Alert type="error" showIcon title={parseApiError(summary.error)} />
        ) : summary.isLoading ? (
          <Spin />
        ) : summary.data ? (
          <>
            <Descriptions
              size="small"
              column={{ xs: 1, sm: 3, md: 3 }}
              items={[
                ["doc_num", "documents"],
                ["chunk_num", "chunks"],
                ["token_num", "tokens"],
              ].map(([key, label]) => ({
                key,
                label: text(label as "documents" | "chunks" | "tokens"),
                children: showKnowledgeValue(summary.data[key]),
              }))}
            />
            <Space wrap>
              {(
                [
                  "unstart_count",
                  "running_count",
                  "cancel_count",
                  "done_count",
                  "fail_count",
                ] as const
              ).map((key, index) => (
                <Tag key={key}>
                  {text(statusKeys[index])}: {showKnowledgeValue(status?.[key])}
                </Tag>
              ))}
            </Space>
          </>
        ) : (
          <Empty description={text("noData")} />
        )}
      </Card>
      <KnowledgeLogs id={id} />
      <Collapse
        items={[
          {
            key: "indexes",
            label: text("indexes"),
            children: (
              <Space
                orientation="vertical"
                size="middle"
                style={{ width: "100%" }}
              >
                <Typography.Text type="secondary">
                  {text("indexHint")}
                </Typography.Text>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns:
                      "repeat(auto-fit, minmax(min(100%, 280px), 1fr))",
                    gap: 12,
                  }}
                >
                  {(["graph", "raptor", "mindmap"] as const).map((kind) => (
                    <KnowledgeIndexCard
                      key={`${id}:${kind}`}
                      id={id}
                      kind={kind}
                    />
                  ))}
                </div>
              </Space>
            ),
          },
          {
            key: "graph",
            label: text("graph"),
            children: <KnowledgeGraph id={id} />,
          },
        ]}
      />
    </Space>
  );
}

export default function KnowledgeIngestionPanel({ id }: { id: string }) {
  return <KnowledgeIngestionWorkspace key={id} id={id} />;
}
