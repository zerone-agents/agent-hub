import { homedir } from "node:os";
import { join } from "node:path";
import { mkdir, readFile, writeFile, chmod } from "node:fs/promises";
import { parse, stringify } from "yaml";

export interface Profile {
  serverUrl: string;
  token: string;
}

export interface Config {
  currentProfile: string;
  profiles: Record<string, Profile>;
}

// Honor process.env.HOME at call time so tests (and users) can override.
// node:os.homedir() ignores HOME changes made after process start on darwin.
function home(): string {
  return process.env.HOME || homedir();
}

export function configPath(): string {
  return join(home(), ".zhub", "config.yaml");
}

export async function loadConfig(): Promise<Config> {
  try {
    const raw = await readFile(configPath(), "utf-8");
    return parse(raw) as Config;
  } catch {
    return { currentProfile: "default", profiles: {} };
  }
}

export async function saveConfig(cfg: Config): Promise<void> {
  const dir = join(home(), ".zhub");
  await mkdir(dir, { recursive: true });
  await writeFile(configPath(), stringify(cfg), "utf-8");
  await chmod(configPath(), 0o600);
}

export async function getActiveProfile(): Promise<Profile> {
  const cfg = await loadConfig();
  const p = cfg.profiles[cfg.currentProfile];
  if (!p) {
    process.stderr.write("未登录。请先运行：zhub login --url <server> --token <cli_xxx>\n");
    process.exit(3);
  }
  return p;
}
