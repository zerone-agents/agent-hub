import { stringify } from "yaml";
import { writeStdout } from "./stdout";

export function outputYaml(data: unknown): void {
  writeStdout(stringify(data) + "\n");
}
