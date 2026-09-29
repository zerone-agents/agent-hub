export const VALID_OUTPUTS = ["table", "json", "yaml"];

export function validateOutput(output: string): number | null {
  if (!VALID_OUTPUTS.includes(output)) {
    process.stderr.write(`Error: --output must be one of ${VALID_OUTPUTS.join(" / ")}\n`);
    return 2;
  }
  return null;
}
