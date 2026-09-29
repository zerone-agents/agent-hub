import { Command, Option } from "clipanion";
import { loadConfig, saveConfig } from "../config";

export class LoginCommand extends Command {
  static paths = [["login"]];
  static usage = Command.Usage({
    description: "Login (writes to ~/.zhub/config.yaml). Must pass --url and --token explicitly, no interactive input",
  });

  url = Option.String("--url", { required: true });
  token = Option.String("--token", { required: true });
  profile = Option.String("--profile", "default");

  async execute(): Promise<number> {
    if (!this.token.startsWith("cli_")) {
      process.stderr.write("Error: token must start with 'cli_'. Generate one in the Hub web console (CLI Tokens page).\n");
      return 2;
    }
    const cfg = await loadConfig();
    cfg.profiles[this.profile] = { serverUrl: this.url, token: this.token };
    cfg.currentProfile = this.profile;
    await saveConfig(cfg);
    console.log(`Logged in to profile "${this.profile}" (${this.url})`);
    return 0;
  }
}
