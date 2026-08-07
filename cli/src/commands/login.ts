import { Command, Option } from "clipanion";
import { loadConfig, saveConfig } from "../config";

export class LoginCommand extends Command {
  static paths = [["login"]];
  static usage = Command.Usage({
    description: "登录（写入 ~/.zhub/config.yaml）。必须显式传 --url 和 --token，无交互输入",
  });

  url = Option.String("--url", { required: true });
  token = Option.String("--token", { required: true });
  profile = Option.String("--profile", "default");

  async execute(): Promise<number> {
    if (!this.token.startsWith("cli_")) {
      process.stderr.write("错误：token 必须以 'cli_' 开头。请到 control-panel 网页「CLI Tokens」页面生成。\n");
      return 2;
    }
    const cfg = await loadConfig();
    cfg.profiles[this.profile] = { serverUrl: this.url, token: this.token };
    cfg.currentProfile = this.profile;
    await saveConfig(cfg);
    console.log(`已登录到 profile "${this.profile}" (${this.url})`);
    return 0;
  }
}
