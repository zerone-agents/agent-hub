import { describe, it, expect } from "vitest";
import { metadataFilterFromJSON } from "./retrievalFilter";

describe("retrieval metadata contract", () => {
  it("keeps numeric zero, lists and empty operators in manual OR conditions", () => {
    expect(
      metadataFilterFromJSON(
        JSON.stringify({
          method: "manual",
          logic: "or",
          manual: [
            { key: " revision ", op: ">=", value: 0 },
            { key: "category", op: "in", value: ["a", "b"] },
            { key: "author", op: "empty" },
          ],
        }),
      ),
    ).toEqual({
      method: "manual",
      logic: "or",
      manual: [
        { key: "revision", op: "≥", value: 0 },
        { key: "category", op: "in", value: ["a", "b"] },
        { key: "author", op: "empty", value: "" },
      ],
    });
  });
  it("supports generated filters and canonicalizes selected-field operators", () => {
    expect(metadataFilterFromJSON('{"method":"auto"}')).toEqual({
      method: "auto",
    });
    expect(
      metadataFilterFromJSON(
        '{"method":"semi_auto","semi_auto":["category",{"key":"revision","op":">="}]}',
      ),
    ).toEqual({
      method: "semi_auto",
      semi_auto: ["category", { key: "revision", op: "≥" }],
    });
  });
  it.each(["", "{}", '{"method":"manual","manual":[]}'])(
    "recognizes intentionally unrestricted metadata: %s",
    (value) => {
      expect(metadataFilterFromJSON(value)).toBeUndefined();
    },
  );
  it.each([
    '{"method":"semi_auto","semi_auto":[]}',
    '{"method":"manual","manual":[{"key":"","op":"=","value":"v2"}]}',
    '{"method":"manual","manual":[{"key":"version","op":"typo","value":"v2"}]}',
    '{"method":"manual","manual":[{"key":"version","op":"in","value":[]}]}',
    '{"logic":"xor","conditions":[]}',
    '{"method":"manual","manual":[{"key":"version","op":"="}]}',
    '{"method":"manual","manual":[{"key":"version","op":"=","value":null}]}',
    '{"conditions":[{"name":"version","comparison_operator":">","value":null}]}',
    "[]",
  ])(
    "rejects an invalid draft instead of silently removing a filter: %s",
    (value) => {
      expect(() => metadataFilterFromJSON(value)).toThrow();
    },
  );
});

it("rejects every unknown semi-auto key, including mixed known/unknown fields", () => {
  for (const fields of [
    ["missing"],
    ["category", "missing"],
    [{ key: "category" }, { key: "missing" }],
  ]) {
    expect(() =>
      metadataFilterFromJSON(
        JSON.stringify({ method: "semi_auto", semi_auto: fields }),
        ["category"],
      ),
    ).toThrow("unknownMetadataFields");
  }
  expect(() =>
    metadataFilterFromJSON(
      '{"method":"semi_auto","semi_auto":["category"]}',
      [],
    ),
  ).toThrow("unknownMetadataFields");
  expect(
    metadataFilterFromJSON('{"method":"semi_auto","semi_auto":["category"]}', [
      "category",
    ]),
  ).toEqual({ method: "semi_auto", semi_auto: ["category"] });
});

const invalidListJSON = [
  "[null]",
  "[{}]",
  '[["ops"]]',
  "[1e309]",
  "[]",
  '["ops",null]',
  "[0,{}]",
  '[false,["ops"]]',
];
it.each(invalidListJSON)(
  "rejects every unsupported element in in/not in and other list values: %s",
  (list) => {
    for (const op of ["in", "not in", "="]) {
      expect(() =>
        metadataFilterFromJSON(
          `{"method":"manual","manual":[{"key":"category","op":"${op}","value":${list}}]}`,
        ),
      ).toThrow("invalidMetadataList");
    }
  },
);
it("preserves typed and mixed positive membership without converting values", () => {
  for (const values of [[0], [false], [true], [-3.5], ["ops", 0, false]]) {
    expect(
      metadataFilterFromJSON(
        JSON.stringify({
          method: "manual",
          manual: [{ key: "category", op: "in", value: values }],
        }),
      )?.manual?.[0]?.value,
    ).toEqual(values);
  }
});
it("continues to block negative membership for typed and text values", () => {
  for (const values of [[0], [false], ["ops", 0, false], ["ops"]]) {
    expect(() =>
      metadataFilterFromJSON(
        JSON.stringify({
          method: "manual",
          manual: [{ key: "category", op: "not in", value: values }],
        }),
      ),
    ).toThrow("unsupportedMetadataExclusion");
  }
});
it.each([
  "=",
  "≠",
  "contains",
  "not contains",
  ">",
  "<",
  "≥",
  "≤",
  "start with",
  "end with",
])("rejects unsupported array comparison %s", (op) => {
  expect(() =>
    metadataFilterFromJSON(
      JSON.stringify({
        method: "manual",
        manual: [{ key: "category", op, value: ["ops"] }],
      }),
    ),
  ).toThrow("unsupportedMetadataList");
});
it("applies membership restrictions to legacy JSON too", () => {
  expect(() =>
    metadataFilterFromJSON(
      '{"conditions":[{"name":"revision","comparison_operator":"not in","value":[0]}]}',
    ),
  ).toThrow("unsupportedMetadataExclusion");
});
it("uses explicit empty operators instead of an empty exclusion list", () => {
  expect(
    metadataFilterFromJSON(
      '{"method":"manual","manual":[{"key":"category","op":"empty","value":[]}]}',
    )?.manual,
  ).toEqual([{ key: "category", op: "empty", value: "" }]);
  expect(
    metadataFilterFromJSON('{"method":"manual","manual":[]}'),
  ).toBeUndefined();
});

it.each(["and", "or"])(
  "rejects the complete %s filter when text not in could retain Alice/Bob documents",
  (logic) => {
    const conditions = [
      { key: "author", op: "not in", value: ["Alice"] },
      { key: "category", op: "in", value: ["ops"] },
    ];
    for (const raw of [
      { method: "manual", logic, manual: conditions },
      {
        logic,
        conditions: conditions.map(({ key, op, value }) => ({
          name: key,
          comparison_operator: op,
          value,
        })),
      },
    ]) {
      expect(() => metadataFilterFromJSON(JSON.stringify(raw))).toThrow(
        "unsupportedMetadataExclusion",
      );
    }
  },
);
it("blocks an explicit semi-auto exclusion before model inference", () => {
  expect(() =>
    metadataFilterFromJSON(
      '{"method":"semi_auto","semi_auto":[{"key":"author","op":"not in"}]}',
      ["author"],
    ),
  ).toThrow("unsupportedMetadataExclusion");
});

it.each(["≠", "not is", "!=", "not contains"])(
  "blocks equivalent negative predicates and aliases: %s",
  (op) => {
    for (const raw of [
      { method: "manual", manual: [{ key: "author", op, value: "Alice" }] },
      {
        conditions: [
          { name: "author", comparison_operator: op, value: "Alice" },
        ],
      },
      { method: "semi_auto", semi_auto: [{ key: "author", op }] },
    ]) {
      expect(() =>
        metadataFilterFromJSON(JSON.stringify(raw), ["author"]),
      ).toThrow("unsupportedMetadataExclusion");
    }
  },
);
