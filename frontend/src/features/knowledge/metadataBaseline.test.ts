import { expect, it } from "vitest";
import {
  baselineMetadataConditionAllowed,
  baselineMetadataDocumentAllowed,
  baselineMetadataFieldAllowed,
} from "./metadataBaseline";
it("keeps text membership available without treating 0, false or mixed fields as typed candidates", () => {
  const fields = [
    { key: "author", type: "string" },
    { key: "revision", type: "number" },
    { key: "flags", type: "boolean" },
  ];
  const flattened = {
    author: { Alice: ["d"], Bob: ["e"] },
    revision: { "0": ["d"] },
    flags: { False: ["d"] },
  };
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "in", value: ["Alice", "Bob"] },
      fields,
      flattened,
    ),
  ).toBe(true);
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "not in", value: [0, false] },
      fields,
      flattened,
    ),
  ).toBe(false);
  expect(
    baselineMetadataConditionAllowed(
      { name: "revision", comparison_operator: "=", value: "0" },
      fields,
      flattened,
    ),
  ).toBe(false);
  expect(
    baselineMetadataConditionAllowed(
      { name: "flags", comparison_operator: "=", value: "False" },
      fields,
      flattened,
    ),
  ).toBe(false);
  expect(
    baselineMetadataFieldAllowed(undefined, { Alice: ["d"], False: ["e"] }),
  ).toBe(false);
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "empty", value: "" },
      fields,
      flattened,
    ),
  ).toBe(false);
});
it("rejects historical nontext values returned for a text condition before previewing mutations", () => {
  expect(
    baselineMetadataDocumentAllowed({ author: ["Alice", "Bob"] }, [
      { name: "author" },
    ]),
  ).toBe(true);
  for (const author of [0, false, null, ["Alice", 0]])
    expect(
      baselineMetadataDocumentAllowed({ author }, [{ name: "author" }]),
    ).toBe(false);
});
it("offers typed date conditions only for the committed strict date representation", () => {
  expect(
    baselineMetadataFieldAllowed({ type: "time" }, { "2026-10-05": ["d"] }),
  ).toBe(true);
  expect(
    baselineMetadataFieldAllowed(
      { type: "time" },
      { "2026-10-05T10:00:00+08:00": ["d"] },
    ),
  ).toBe(false);
});
import { baselineMetadataEquivalent } from "./metadataBaseline";
it("detects type-only replacements the committed batch writer skips without blocking changes to other fields", () => {
  expect(baselineMetadataEquivalent({ flags: false }, { flags: 0 })).toBe(true);
  expect(baselineMetadataEquivalent({ flags: [true] }, { flags: [1] })).toBe(
    true,
  );
  expect(
    baselineMetadataEquivalent(
      { flags: false, author: "A" },
      { flags: 0, author: "B" },
    ),
  ).toBe(false);
});
it.each([
  "0x0",
  "0b0",
  "0o0",
  "-0xA",
  "1_000",
  "1j",
  "1 + 2j",
  "+ 0x0",
  "True #comment",
  "#comment\nFalse",
  "b'1'",
  "r'A'",
  "...",
  "set()",
  "+ 0x0",
  "b'#hash'",
  "#header\nb'#hash'",
])(
  "rejects Python-literal text %s for scalar comparisons without banning text membership",
  (value) => {
    const fields = [{ key: "author", type: "string" }];
    const flattened = { author: { Alice: ["d"] } };
    expect(
      baselineMetadataConditionAllowed(
        { name: "author", comparison_operator: "=", value },
        fields,
        flattened,
      ),
    ).toBe(false);
    expect(
      baselineMetadataConditionAllowed(
        { name: "author", comparison_operator: "in", value: [value] },
        fields,
        { author: { [value]: ["d"] } },
      ),
    ).toBe(true);
  },
);
it("keeps ordinary text and rejects known or discovered list exclusions", () => {
  const fields = [{ key: "author", type: "string" }];
  const flattened = { author: { "10 reasons": ["d"], Alice: ["e"] } };
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "=", value: "10 reasons" },
      fields,
      flattened,
    ),
  ).toBe(true);
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "not in", value: ["Alice"] },
      [{ key: "author", type: "list" }],
      flattened,
    ),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: ["Alice", "Bob"] }, [
      { name: "author", comparison_operator: "not in", value: ["Alice"] },
    ]),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: ["Alice", "Bob"] }, [
      { name: "author", comparison_operator: "in", value: ["Alice"] },
    ]),
  ).toBe(true);
  expect(
    baselineMetadataDocumentAllowed({ author: "Bob" }, [
      { name: "author", comparison_operator: "not in", value: ["Alice"] },
    ]),
  ).toBe(false);
});

it("checks actual predicates with complete AND/OR and rejects post-inventory literal aliases", () => {
  const conditions = [
    { name: "author", comparison_operator: "=", value: "Alice" },
    { name: "topic", comparison_operator: "=", value: "Research" },
  ];
  expect(
    baselineMetadataDocumentAllowed({ author: "Alice" }, conditions, "or"),
  ).toBe(true);
  expect(
    baselineMetadataDocumentAllowed({ author: "Alice" }, conditions, "and"),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed(
      { author: "Bob", topic: "Other" },
      conditions,
      "or",
    ),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: "r'Alice'" }, [conditions[0]]),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: "0b0" }, [
      { name: "author", comparison_operator: "=", value: "0x0" },
    ]),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: "0b0" }, [
      { name: "author", comparison_operator: "in", value: ["0x0"] },
    ]),
  ).toBe(false);
  expect(
    baselineMetadataDocumentAllowed({ author: "0x0" }, [
      { name: "author", comparison_operator: "in", value: ["0x0"] },
    ]),
  ).toBe(true);
});

it.each(["not in", "not contains", "≠", "!=", "not is"])(
  "rejects negative JSON alias %s while preserving positive is",
  (op) => {
    const fields = [{ key: "author", type: "string" }];
    const flattened = { author: { Alice: ["d1"], Bob: ["d1", "d2"] } };
    expect(
      baselineMetadataConditionAllowed(
        {
          name: "author",
          comparison_operator: op,
          value: op === "not in" ? ["Alice"] : "Alice",
        },
        fields,
        flattened,
      ),
    ).toBe(false);
    expect(
      baselineMetadataDocumentAllowed({ author: "Bob" }, [
        { name: "author", comparison_operator: op, value: "Alice" },
      ]),
    ).toBe(false);
    expect(
      baselineMetadataConditionAllowed(
        { name: "author", comparison_operator: "is", value: "Alice" },
        fields,
        flattened,
      ),
    ).toBe(true);
  },
);

it.each([
  "Bob's note",
  "buff's profile",
  "r's graph",
  "10 reasons",
  "Release v1.2",
])("retains common plain text %s", (value) => {
  expect(
    baselineMetadataConditionAllowed(
      { name: "author", comparison_operator: "=", value },
      [{ key: "author", type: "string" }],
      { author: { [value]: ["d"] } },
    ),
  ).toBe(true);
});
