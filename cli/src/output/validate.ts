export const VALID_OUTPUTS = ["table", "json", "yaml"];

export function validateOutput(output: string): number | null {
  if (!VALID_OUTPUTS.includes(output)) {
    process.stderr.write(`错误：--output 必须是 ${VALID_OUTPUTS.join(" / ")} 之一\n`);
    return 2;
  }
  return null;
}
