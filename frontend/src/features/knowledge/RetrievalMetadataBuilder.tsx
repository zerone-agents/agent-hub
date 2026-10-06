import {
  Alert,
  AutoComplete,
  Button,
  Collapse,
  Input,
  InputNumber,
  Select,
  Space,
  Typography,
} from "antd";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { createStyles } from "antd-style";
import {
  FILTER_OPERATORS,
  isEmptyOperator,
  isUnsupportedExclusionOperator,
  metadataFilterFromJSON,
  retrievalMetadataKey,
  loadRetrievalMetadataKeys,
} from "./retrievalFilter";
import "@/i18n/locales/retrievalWorkbench";

const useStyles = createStyles(({ css }) => ({
  row: css`
    display: grid;
    grid-template-columns: minmax(120px, 1fr) minmax(120px, 1fr) auto;
    gap: 8px;
    margin: 12px 0;
    padding: 12px;
    background: var(--background);
    border-radius: 8px;
    .value {
      grid-column: 1 / -1;
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    @container retrieval (max-width: 580px) {
      grid-template-columns: minmax(0, 1fr) auto;
      .operator {
        grid-column: 1;
        grid-row: 2;
      }
    }
  `,
}));
interface Row {
  key: string;
  op?: string;
  value?: unknown;
}
type Mode = "disabled" | "manual" | "auto" | "semi_auto";
const listToken = (value: unknown) => JSON.stringify([typeof value, value]);
const listLabel = (value: unknown) =>
  typeof value === "string"
    ? value
    : typeof value === "number" || typeof value === "boolean"
      ? String(value)
      : JSON.stringify(value);
export default function RetrievalMetadataBuilder({
  datasetId,
  value = "",
  onChange,
  id,
}: {
  datasetId: string;
  value?: string;
  onChange?: (value: string) => void;
  id?: string;
}) {
  const { t } = useTranslation("retrievalWorkbench");
  const { styles } = useStyles();
  const fields = useQuery({
    queryKey: retrievalMetadataKey(datasetId),
    queryFn: () => loadRetrievalMetadataKeys(datasetId),
    retry: false,
  });
  let mode: Mode = "disabled";
  let logic: "and" | "or" = "and";
  let rows: Row[] = [];
  let invalid = false;
  let invalidMessage = "invalid";
  try {
    metadataFilterFromJSON(value, fields.data);
  } catch (error) {
    invalid = true;
    if (
      error instanceof Error &&
      [
        "invalidMetadataList",
        "unsupportedMetadataList",
        "unsupportedMetadataExclusion",
      ].includes(error.message)
    )
      invalidMessage = error.message;
  }
  try {
    const raw = JSON.parse(value || "{}") as Record<string, unknown>;
    mode = (raw.method ?? (raw.conditions ? "manual" : "disabled")) as Mode;
    logic = raw.logic === "or" ? "or" : "and";
    const entries =
      mode === "semi_auto"
        ? raw.semi_auto
        : raw.method
          ? raw.manual
          : raw.conditions;
    if (Array.isArray(entries))
      rows = entries.map((entry: unknown) => {
        if (typeof entry === "string") return { key: entry };
        if (!entry || typeof entry !== "object") return { key: "" };
        const item = entry as Record<string, unknown>;
        return {
          key:
            typeof (item.key ?? item.name) === "string"
              ? ((item.key ?? item.name) as string)
              : "",
          op:
            typeof (item.op ?? item.comparison_operator) === "string"
              ? ((item.op ?? item.comparison_operator) as string)
              : "",
          value: item.value,
        };
      });
  } catch {
    /* Keep invalid JSON untouched until an explicit builder edit. */
  }
  const update = (nextMode: Mode, nextRows = rows, nextLogic = logic) => {
    onChange?.(
      nextMode === "disabled"
        ? ""
        : JSON.stringify(
            {
              method: nextMode,
              ...(nextMode === "manual"
                ? { logic: nextLogic, manual: nextRows }
                : {}),
              ...(nextMode === "semi_auto"
                ? {
                    semi_auto: nextRows.map(({ key, op }) => ({
                      key,
                      ...(op ? { op } : {}),
                    })),
                  }
                : {}),
            },
            null,
            2,
          ),
    );
  };
  const changeRow = (index: number, patch: Partial<Row>) => {
    update(
      mode,
      rows.map((row, n) => (n === index ? { ...row, ...patch } : row)),
    );
  };
  const operatorKeys: Record<string, string> = {
    "=": "equals",
    "≠": "notEquals",
    contains: "contains",
    "not contains": "notContains",
    "start with": "starts",
    "end with": "ends",
    empty: "empty",
    "not empty": "notEmpty",
    in: "in",
    "not in": "notIn",
  };
  const operators = FILTER_OPERATORS.map((op) => ({
    value: op,
    label: operatorKeys[op] ? t(`operators.${operatorKeys[op]}`) : op,
    disabled: isUnsupportedExclusionOperator(op),
  }));
  return (
    <div>
      <Select
        id={id}
        aria-label={t("mode")}
        style={{ width: "100%" }}
        value={mode}
        options={(["disabled", "manual", "auto", "semi_auto"] as const).map(
          (option) => ({ value: option, label: t(option) }),
        )}
        onChange={(next: Mode) => {
          update(
            next,
            next === "manual" || next === "semi_auto"
              ? [
                  {
                    key: "",
                    ...(next === "manual" ? { op: "=", value: "" } : {}),
                  },
                ]
              : [],
          );
        }}
      />
      {(mode === "auto" || mode === "semi_auto") && (
        <>
          <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
            {t(mode === "auto" ? "autoHint" : "semiHint")}
          </Typography.Paragraph>
          <Alert type="warning" showIcon title={t("generatedExclusionHint")} />
        </>
      )}
      {mode === "manual" && (
        <Space wrap style={{ marginTop: 12 }}>
          <Typography.Text>{t("logic")}</Typography.Text>
          <Select
            aria-label={t("logic")}
            value={logic}
            options={["and", "or"].map((option) => ({
              value: option,
              label: t(option),
            }))}
            onChange={(next: "and" | "or") => {
              update(mode, rows, next);
            }}
          />
        </Space>
      )}
      {(mode === "manual" || mode === "semi_auto") && (
        <>
          {fields.error && (
            <Alert
              type="warning"
              showIcon
              title={t("keysError")}
              action={
                <Button
                  size="small"
                  onClick={() => {
                    void fields.refetch();
                  }}
                >
                  {t("retry")}
                </Button>
              }
            />
          )}
          {rows.map((row, index) => (
            <div className={styles.row} key={index}>
              {mode === "semi_auto" ? (
                <Select
                  aria-label={`${t("field")} ${index + 1}`}
                  placeholder={t("field")}
                  loading={fields.isFetching}
                  showSearch={{ optionFilterProp: "label" }}
                  value={row.key === "" ? undefined : row.key}
                  status={
                    row.key && fields.data && !fields.data.includes(row.key)
                      ? "error"
                      : undefined
                  }
                  options={(fields.data ?? []).map((key) => ({
                    value: key,
                    label: key,
                  }))}
                  onChange={(key) => {
                    changeRow(index, { key });
                  }}
                />
              ) : (
                <AutoComplete
                  value={row.key}
                  options={(fields.data ?? []).map((key) => ({ value: key }))}
                  showSearch={{
                    filterOption: (input, option) =>
                      (option?.value ?? "")
                        .toLowerCase()
                        .includes(input.toLowerCase()),
                  }}
                  onChange={(key) => {
                    changeRow(index, { key });
                  }}
                >
                  <Input
                    aria-label={`${t("field")} ${index + 1}`}
                    placeholder={t("field")}
                  />
                </AutoComplete>
              )}
              <Select
                className="operator"
                virtual={false}
                aria-label={`${t("operator")} ${index + 1}`}
                value={row.op === "" ? undefined : row.op}
                placeholder={t("modelOperator")}
                options={
                  mode === "semi_auto"
                    ? [{ value: "", label: t("modelOperator") }, ...operators]
                    : operators
                }
                onChange={(op) => {
                  changeRow(index, {
                    op,
                    ...(op === "in" || op === "not in"
                      ? { value: Array.isArray(row.value) ? row.value : [] }
                      : {}),
                  });
                }}
              />
              <Button
                aria-label={`${t("remove")} ${index + 1}`}
                onClick={() => {
                  update(
                    mode,
                    rows.filter((_, n) => n !== index),
                  );
                }}
              >
                ×
              </Button>
              {mode === "manual" && !isEmptyOperator(row.op ?? "") && (
                <div className="value">
                  <Select
                    aria-label={`${t("valueType")} ${index + 1}`}
                    style={{ width: 130 }}
                    value={
                      Array.isArray(row.value)
                        ? "list"
                        : typeof row.value === "number" || row.value === null
                          ? "number"
                          : "text"
                    }
                    disabled={row.op === "in" || row.op === "not in"}
                    options={(row.op === "in" || row.op === "not in"
                      ? ["list"]
                      : ["text", "number"]
                    ).map((type) => ({
                      value: type,
                      label: t(type),
                    }))}
                    onChange={(type) => {
                      changeRow(index, {
                        value:
                          type === "number" ? 0 : type === "list" ? [] : "",
                      });
                    }}
                  />
                  {Array.isArray(row.value) ? (
                    <Select
                      aria-label={`${t("value")} ${index + 1}`}
                      mode="tags"
                      style={{ flex: 1, minWidth: 100 }}
                      value={row.value.map(listToken)}
                      options={row.value.map((item) => ({
                        value: listToken(item),
                        label: listLabel(item),
                      }))}
                      onChange={(list) => {
                        const existing = new Map(
                          (row.value as unknown[]).map((item) => [
                            listToken(item),
                            item,
                          ]),
                        );
                        changeRow(index, {
                          value: list.map((token: string) =>
                            existing.has(token) ? existing.get(token) : token,
                          ),
                        });
                      }}
                    />
                  ) : typeof row.value === "number" || row.value === null ? (
                    <InputNumber
                      aria-label={`${t("value")} ${index + 1}`}
                      style={{ flex: 1, minWidth: 100 }}
                      value={row.value}
                      onChange={(number) => {
                        changeRow(index, { value: number });
                      }}
                    />
                  ) : (
                    <Input
                      aria-label={`${t("value")} ${index + 1}`}
                      style={{ flex: 1, minWidth: 100 }}
                      value={typeof row.value === "string" ? row.value : ""}
                      onChange={(event) => {
                        changeRow(index, { value: event.target.value });
                      }}
                    />
                  )}
                  {Array.isArray(row.value) && (
                    <Typography.Text type="secondary">
                      {t("listHint")}
                    </Typography.Text>
                  )}
                </div>
              )}
            </div>
          ))}
          <Button
            onClick={() => {
              update(mode, [
                ...rows,
                {
                  key: "",
                  ...(mode === "manual" ? { op: "=", value: "" } : {}),
                },
              ]);
            }}
          >
            {t("add")}
          </Button>
        </>
      )}
      {invalid && (
        <Typography.Paragraph
          type="danger"
          style={{ marginTop: 8 }}
          role="alert"
        >
          {t(invalidMessage)}
        </Typography.Paragraph>
      )}
      <Collapse
        ghost
        style={{ marginTop: 8 }}
        items={[
          {
            key: "json",
            label: t("advancedJSON"),
            children: (
              <Input.TextArea
                aria-label={t("advancedJSON")}
                rows={5}
                value={value}
                onChange={(event) => onChange?.(event.target.value)}
              />
            ),
          },
        ]}
      />
    </div>
  );
}
