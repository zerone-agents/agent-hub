import { stringify } from "yaml";
export function outputYaml(data: unknown): void {
  console.log(stringify(data));
}
