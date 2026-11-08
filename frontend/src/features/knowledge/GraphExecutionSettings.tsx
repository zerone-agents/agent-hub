import { Col, InputNumber, Row, Typography } from "antd";
import { graphBudgetRules } from "./datasetParserValidation";
import { useParserTranslation } from "./useParserTranslation";
export default function GraphExecutionSettings({
  value = {},
  onChange,
  disabled,
}: {
  value?: Record<string, number>;
  onChange?: (value: Record<string, number>) => void;
  disabled?: boolean;
}) {
  const { t } = useParserTranslation();
  return (
    <>
      <Typography.Paragraph type="secondary">
        {t("graphExecutionHint")}
      </Typography.Paragraph>
      <Row gutter={[12, 12]}>
        {Object.entries(graphBudgetRules).map(
          ([key, [min, max, integer = true]]) => (
            <Col xs={24} md={12} key={key}>
              <label style={{ display: "block" }}>
                <Typography.Text>{t(`graphBudgets.${key}`)}</Typography.Text>
                <InputNumber
                  aria-label={t(`graphBudgets.${key}`)}
                  style={{ width: "100%" }}
                  value={value[key]}
                  min={min}
                  max={max}
                  precision={integer ? 0 : undefined}
                  disabled={disabled}
                  placeholder={t("serverDefault")}
                  onChange={(next) => {
                    if (typeof next === "number")
                      onChange?.({ ...value, [key]: next });
                  }}
                />
              </label>
            </Col>
          ),
        )}
      </Row>
    </>
  );
}
