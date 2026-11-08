import { useEffect, useState } from "react";
import { TrashIcon } from "@phosphor-icons/react";
import { Alert, Button, Input, Select, Space, Typography } from "antd";
import { useParserTranslation } from "./useParserTranslation";
import {
  buildTableUploadConfig,
  validateDatasetTableConfig,
  type TableColumnRole,
  type TableDatasetConfig,
  type TableUploadConfig,
} from "./tableParserConfig";
import { extractTableHeaders } from "./tableHeaders";

function RoleControls({
  columns,
  roles,
  disabled,
  onChange,
  labelPrefix = "",
  onRemove,
}: {
  columns: string[];
  roles: TableDatasetConfig["table_column_roles"];
  disabled?: boolean;
  labelPrefix?: string;
  onChange: (roles: Record<string, TableColumnRole>) => void;
  onRemove?: (column: string) => void;
}) {
  const { t } = useParserTranslation();
  return (
    <div
      style={{ maxHeight: 300, overflowY: "auto" }}
      role="group"
      aria-label={t("roles")}
    >
      {columns.map((column) => (
        <div
          key={column}
          style={{
            display: "flex",
            gap: 12,
            alignItems: "center",
            marginBottom: 8,
          }}
        >
          <Typography.Text
            style={{
              flex: 1,
              minWidth: 0,
              overflowWrap: "anywhere",
              whiteSpace: "pre-wrap",
            }}
          >
            {column}
          </Typography.Text>
          <Select
            aria-label={`${labelPrefix}${column}`}
            style={{ width: onRemove ? 156 : 180, flexShrink: 0 }}
            disabled={disabled}
            value={
              roles && Object.hasOwn(roles, column)
                ? roles[column] === "vectorize"
                  ? "indexing"
                  : roles[column]
                : "both"
            }
            options={["both", "indexing", "metadata"].map((value) => ({
              value,
              label: t(value),
            }))}
            onChange={(role: TableColumnRole) => {
              onChange(
                Object.fromEntries<TableColumnRole>([
                  ...Object.entries(roles ?? {}).map(
                    ([key, value]): [string, TableColumnRole] => [
                      key,
                      value === "vectorize" ? "indexing" : value,
                    ],
                  ),
                  [column, role],
                ]),
              );
            }}
          />
          {onRemove && (
            <Button
              size="small"
              type="text"
              disabled={disabled}
              aria-label={`${labelPrefix}${t("removeColumn")}: ${column}`}
              icon={<TrashIcon size={14} />}
              onClick={() => {
                onRemove(column);
              }}
            />
          )}
        </div>
      ))}
    </div>
  );
}
export function DatasetTableSettings({
  value = {},
  onChange,
  disabled,
}: {
  value?: TableDatasetConfig;
  onChange?: (value: TableDatasetConfig) => void;
  disabled?: boolean;
}) {
  const { t } = useParserTranslation();
  try {
    validateDatasetTableConfig(value as Record<string, unknown>);
  } catch {
    return <Alert type="warning" title={t("invalidStored")} />;
  }
  const columns = [
    ...new Set([
      ...(value.table_column_names ?? []),
      ...Object.keys(value.table_column_roles ?? {}),
    ]),
  ];
  return (
    <Space orientation="vertical" style={{ width: "100%" }}>
      <Typography.Paragraph type="secondary">
        {t("datasetHint")}
      </Typography.Paragraph>
      <Select
        aria-label={t("mode")}
        style={{ width: "100%" }}
        disabled={disabled}
        value={value.table_column_mode ?? "auto"}
        options={["auto", "manual"].map((value) => ({
          value,
          label: t(value),
        }))}
        onChange={(mode: "auto" | "manual") =>
          onChange?.({ ...value, table_column_mode: mode })
        }
      />
      {value.table_column_mode === "manual" &&
        (columns.length ? (
          <RoleControls
            columns={columns}
            roles={value.table_column_roles}
            disabled={disabled}
            onChange={(roles) =>
              onChange?.({ ...value, table_column_roles: roles })
            }
          />
        ) : (
          <Typography.Paragraph type="secondary">
            {t("empty")}
          </Typography.Paragraph>
        ))}
    </Space>
  );
}
export function FileTableSettings({
  file,
  value,
  disabled,
  onChange,
}: {
  file: File;
  value?: TableUploadConfig;
  disabled: boolean;
  onChange: (value?: TableUploadConfig) => void;
}) {
  const { t } = useParserTranslation();
  const [headers, setHeaders] = useState<string[]>();
  const [failed, setFailed] = useState(false);
  const [roles, setRoles] = useState<Record<string, TableColumnRole>>(
    value?.table_column_roles ?? {},
  );
  const [columnDraft, setColumnDraft] = useState("");
  const [columnError, setColumnError] = useState("");
  const updateRoles = (next: Record<string, TableColumnRole>) => {
    if (disabled) return;
    setRoles(next);
    onChange(buildTableUploadConfig("manual", next));
  };
  const addColumn = () => {
    if (disabled) return;
    if (!columnDraft.trim()) {
      setColumnError("columnBlank");
      return;
    }
    if (Object.hasOwn(roles, columnDraft)) {
      setColumnError("columnDuplicate");
      return;
    }
    updateRoles({ ...roles, [columnDraft]: "both" });
    setColumnDraft("");
    setColumnError("");
  };
  useEffect(() => {
    let active = true;
    void extractTableHeaders(file).then(
      (columns) => {
        if (active) setHeaders(columns);
      },
      () => {
        if (active) setFailed(true);
      },
    );
    return () => {
      active = false;
    };
  }, [file]);
  const mode = value?.table_column_mode ?? "inherit";
  return (
    <Space orientation="vertical" style={{ width: "100%", marginTop: 8 }}>
      <Select
        aria-label={`${file.name}: ${t("mode")}`}
        disabled={disabled}
        style={{ width: "100%" }}
        value={mode}
        options={["inherit", "auto", "manual"].map((value) => ({
          value,
          label: t(value),
        }))}
        onChange={(next: "inherit" | "auto" | "manual") => {
          const selected = headers
            ? Object.fromEntries<TableColumnRole>(
                headers.map((name) => [
                  name,
                  Object.hasOwn(roles, name) ? roles[name] : "both",
                ]),
              )
            : roles;
          setRoles(selected);
          onChange(buildTableUploadConfig(next, selected));
        }}
      />
      <Typography.Text type="secondary">
        {t(
          failed ? "headerFailed" : !headers ? "readingHeaders" : "uploadHint",
        )}
      </Typography.Text>
      {mode === "manual" && (
        <>
          <Typography.Text type="secondary">
            {t("manualDefaultsHint")}
          </Typography.Text>
          {failed && (
            <>
              <Typography.Text type="secondary">
                {t("manualNamesHint")}
              </Typography.Text>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                <Input
                  aria-label={`${file.name}: ${t("columnName")}`}
                  placeholder={t("columnNamePlaceholder")}
                  style={{ flex: "1 1 160px", minWidth: 0 }}
                  disabled={disabled}
                  value={columnDraft}
                  status={columnError ? "error" : undefined}
                  onPressEnter={(event) => {
                    event.preventDefault();
                  }}
                  onChange={(event) => {
                    setColumnDraft(event.target.value);
                    setColumnError("");
                  }}
                />
                <Button disabled={disabled} onClick={addColumn}>
                  {t("addColumn")}
                </Button>
              </div>
              {columnError && (
                <Typography.Text type="danger" role="alert">
                  {t(columnError)}
                </Typography.Text>
              )}
              {!!columnDraft && (
                <Typography.Text type="secondary">
                  {t("columnDraftHint")}
                </Typography.Text>
              )}
            </>
          )}
          <RoleControls
            columns={headers ?? Object.keys(roles)}
            roles={roles}
            disabled={disabled}
            labelPrefix={`${file.name}: `}
            onChange={updateRoles}
            onRemove={
              failed
                ? (column) => {
                    updateRoles(
                      Object.fromEntries(
                        Object.entries(roles).filter(
                          ([name]) => name !== column,
                        ),
                      ),
                    );
                  }
                : undefined
            }
          />
        </>
      )}
    </Space>
  );
}
