import { useState } from "react";
import {
  Alert,
  Button,
  Descriptions,
  Empty,
  Input,
  Modal,
  Space,
  Table,
  Tree,
  Typography,
} from "antd";
import type { DataNode } from "antd/es/tree";
import type { KnowledgeObject } from "@/api/knowledgeManagement";
import {
  graphEndpoint,
  graphNodeId,
  knowledgeObject,
  knowledgeRows,
  showKnowledgeValue,
} from "./knowledgeDiagnostics";
import { KnowledgeRawData } from "./KnowledgeDiagnosticDetails";
import { useDiagnosticsText } from "./knowledgeDiagnostics.messages";

function mindmapNodes(value: unknown, path = "root", depth = 0): DataNode[] {
  if (depth > 20) return [];
  if (Array.isArray(value))
    return value.flatMap((child: unknown, index) =>
      mindmapNodes(child, `${path}-${index}`, depth + 1),
    );
  const object = knowledgeObject(value);
  if (!object)
    return value === undefined || value === null
      ? []
      : [{ key: path, title: showKnowledgeValue(value) }];
  if (Object.keys(object).length === 0) return [];
  const title = object.name ?? object.title ?? object.id ?? object.label;
  if (title !== undefined)
    return [
      {
        key: path,
        title: showKnowledgeValue(title),
        children: mindmapNodes(object.children, `${path}-children`, depth + 1),
      },
    ];
  return Object.entries(object).map(([key, child]) => ({
    key: `${path}-${key}`,
    title: key,
    children: mindmapNodes(child, `${path}-${key}-child`, depth + 1),
  }));
}

export default function KnowledgeGraphExplorer({
  data,
  loading = false,
  emptyText,
}: {
  data?: KnowledgeObject;
  loading?: boolean;
  emptyText?: string;
}) {
  const text = useDiagnosticsText();
  const [filter, setFilter] = useState("");
  const [selection, setSelection] = useState<{
    type: "node" | "edge";
    object: KnowledgeObject;
  }>();
  const graph = knowledgeObject(data?.graph);
  const allNodes = knowledgeRows(graph?.nodes);
  const allEdges = knowledgeRows(graph?.edges);
  const nodes = allNodes.filter((node) =>
    JSON.stringify(node).toLowerCase().includes(filter.trim().toLowerCase()),
  );
  const nodeIds = new Set(nodes.map(graphNodeId));
  const edges = filter.trim()
    ? allEdges.filter(
        (edge) =>
          nodeIds.has(graphEndpoint(edge, "source")) ||
          nodeIds.has(graphEndpoint(edge, "target")),
      )
    : allEdges;
  const mindmap = mindmapNodes(data?.mind_map);
  const selected = selection?.object;
  const selectedId =
    selected && selection.type === "node" ? graphNodeId(selected) : undefined;
  const neighbours = selectedId
    ? allEdges.filter(
        (edge) =>
          graphEndpoint(edge, "source") === selectedId ||
          graphEndpoint(edge, "target") === selectedId,
      )
    : [];

  function entityLink(id: string) {
    const entity = allNodes.find((node) => graphNodeId(node) === id);
    return entity ? (
      <Button
        type="link"
        style={{
          paddingInline: 0,
          height: "auto",
          whiteSpace: "normal",
          textAlign: "left",
        }}
        aria-label={text("viewEntity", { name: id })}
        onClick={() => {
          setSelection({ type: "node", object: entity });
        }}
      >
        {id}
      </Button>
    ) : (
      <span title={text("missingEntity")}>{id}</span>
    );
  }

  return (
    <Space
      orientation="vertical"
      size="middle"
      style={{ width: "100%", minWidth: 0 }}
    >
      <Input.Search
        allowClear
        value={filter}
        aria-label={text("search")}
        placeholder={text("search")}
        onChange={(event) => {
          setFilter(event.target.value);
        }}
        onSearch={setFilter}
      />
      <Typography.Text type="secondary">{text("filterHint")}</Typography.Text>
      {data && (
        <Typography.Text>
          {text("graphCounts", {
            nodes: allNodes.length,
            edges: allEdges.length,
          })}
        </Typography.Text>
      )}
      {filter.trim() && (
        <Space wrap>
          <Typography.Text>
            {text("visibleCounts", {
              nodes: nodes.length,
              edges: edges.length,
            })}
          </Typography.Text>
          <Button
            onClick={() => {
              setFilter("");
            }}
          >
            {text("reset")}
          </Button>
        </Space>
      )}
      {!loading &&
      data &&
      allNodes.length === 0 &&
      allEdges.length === 0 &&
      mindmap.length === 0 ? (
        <Empty description={emptyText ?? text("graphEmpty")} />
      ) : (
        <>
          <Typography.Text strong>{text("entities")}</Typography.Text>
          <Table<KnowledgeObject>
            size="small"
            rowKey={(_, index) => String(index)}
            dataSource={nodes}
            loading={loading}
            scroll={{ x: 560 }}
            pagination={{ pageSize: 10, hideOnSinglePage: true }}
            locale={{
              emptyText: filter.trim() ? text("noMatches") : text("noData"),
            }}
            columns={[
              {
                title: text("entityName"),
                render: (_, row) => entityLink(graphNodeId(row)),
              },
              {
                title: text("entityType"),
                render: (_, row) =>
                  showKnowledgeValue(row.entity_type ?? row.type),
              },
              {
                title: text("description"),
                render: (_, row) => (
                  <span style={{ overflowWrap: "anywhere" }}>
                    {showKnowledgeValue(row.description)}
                  </span>
                ),
              },
            ]}
          />
          <Typography.Text strong>{text("relations")}</Typography.Text>
          <Table<KnowledgeObject>
            size="small"
            rowKey={(_, index) => String(index)}
            dataSource={edges}
            loading={loading}
            scroll={{ x: 560 }}
            pagination={{ pageSize: 10, hideOnSinglePage: true }}
            columns={[
              {
                title: text("relationSource"),
                render: (_, row) => entityLink(graphEndpoint(row, "source")),
              },
              {
                title: text("relationTarget"),
                render: (_, row) => entityLink(graphEndpoint(row, "target")),
              },
              {
                title: text("description"),
                render: (_, row) => (
                  <Button
                    type="link"
                    style={{
                      paddingInline: 0,
                      whiteSpace: "normal",
                      height: "auto",
                      textAlign: "left",
                    }}
                    aria-label={text("viewRelation", {
                      source: graphEndpoint(row, "source"),
                      target: graphEndpoint(row, "target"),
                    })}
                    onClick={() => {
                      setSelection({ type: "edge", object: row });
                    }}
                  >
                    {showKnowledgeValue(row.description)}
                  </Button>
                ),
              },
            ]}
          />
        </>
      )}
      {mindmap.length > 0 && (
        <>
          <Typography.Text strong>{text("mindmap")}</Typography.Text>
          <Tree
            treeData={mindmap}
            defaultExpandAll
            blockNode
            style={{ background: "transparent", overflow: "auto" }}
          />
        </>
      )}
      {data && <KnowledgeRawData data={data} />}
      <Modal
        open={Boolean(selection)}
        title={text(
          selection?.type === "edge" ? "relationDetails" : "entityDetails",
        )}
        onCancel={() => {
          setSelection(undefined);
        }}
        footer={
          <Button
            onClick={() => {
              setSelection(undefined);
            }}
          >
            {text("close")}
          </Button>
        }
      >
        {selected && (
          <Space orientation="vertical" style={{ width: "100%" }} size="middle">
            <Descriptions
              column={1}
              size="small"
              styles={{ content: { overflowWrap: "anywhere" } }}
              items={Object.entries(selected).map(([key, value]) => ({
                key,
                label: key,
                children: (
                  <span style={{ whiteSpace: "pre-wrap" }}>
                    {showKnowledgeValue(value)}
                  </span>
                ),
              }))}
            />
            {selectedId && (
              <>
                <Typography.Text strong>{text("neighbours")}</Typography.Text>
                {neighbours.length === 0 ? (
                  <Alert type="info" title={text("noNeighbours")} />
                ) : (
                  neighbours.map((edge, index) => (
                    <div key={index} style={{ overflowWrap: "anywhere" }}>
                      {entityLink(graphEndpoint(edge, "source"))} →{" "}
                      {entityLink(graphEndpoint(edge, "target"))}
                      <div>{showKnowledgeValue(edge.description)}</div>
                    </div>
                  ))
                )}
              </>
            )}
          </Space>
        )}
      </Modal>
    </Space>
  );
}
