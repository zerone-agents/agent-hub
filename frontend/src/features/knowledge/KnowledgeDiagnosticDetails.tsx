import { Collapse, Descriptions, Typography } from "antd";
import type { KnowledgeObject } from "@/api/knowledgeManagement";
import { showKnowledgeValue } from "./knowledgeDiagnostics";
import { useDiagnosticsText } from "./knowledgeDiagnostics.messages";

export function KnowledgeRawData({ data }: { data: unknown }) {
  const text = useDiagnosticsText();
  return (
    <Collapse
      size="small"
      items={[
        {
          key: "raw",
          label: text("raw"),
          children: (
            <pre
              style={{
                whiteSpace: "pre-wrap",
                overflowWrap: "anywhere",
                maxHeight: 320,
                overflow: "auto",
                margin: 0,
              }}
            >
              {JSON.stringify(data, null, 2)}
            </pre>
          ),
        },
      ]}
    />
  );
}

export function KnowledgeDiagnosticDetails({
  data,
}: {
  data: KnowledgeObject;
}) {
  const text = useDiagnosticsText();
  return (
    <Descriptions
      column={1}
      size="small"
      items={[
        {
          key: "task",
          label: text("taskId"),
          children: showKnowledgeValue(data.task_id),
        },
        {
          key: "type",
          label: text("type"),
          children: showKnowledgeValue(data.task_type),
        },
        {
          key: "source",
          label: text("source"),
          children: showKnowledgeValue(data.source_from),
        },
        {
          key: "started",
          label: text("started"),
          children: showKnowledgeValue(data.process_begin_at ?? data.begin_at),
        },
        {
          key: "created",
          label: text("created"),
          children: showKnowledgeValue(data.create_date),
        },
        {
          key: "duration",
          label: text("duration"),
          children: showKnowledgeValue(data.process_duration),
        },
        {
          key: "stage",
          label: text("stage"),
          children: (
            <Typography.Paragraph
              style={{
                whiteSpace: "pre-wrap",
                overflowWrap: "anywhere",
                maxHeight: 280,
                overflow: "auto",
                margin: 0,
              }}
            >
              {showKnowledgeValue(data.progress_msg)}
            </Typography.Paragraph>
          ),
        },
      ]}
      styles={{ content: { overflowWrap: "anywhere" } }}
    />
  );
}
