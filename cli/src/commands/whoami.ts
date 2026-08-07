import { Command } from "clipanion";
import { loadConfig } from "../config";
import { apiRequest } from "../client/base";

// Mirrors the data block returned by GET /auth/userinfo
// (see internal/handler/auth.go::UserInfo). The endpoint wraps the payload in
// `{ success: true, data: {...} }`; apiRequest unwraps that envelope.
interface CasdoorRole {
  name?: string;
  displayName?: string;
}

interface UserInfo {
  id: string;
  username?: string;
  email?: string;
  display_name?: string;
  tenant_id?: string;
  org_id?: string;
  avatar?: string;
  roles?: (string | CasdoorRole)[];
  permissions?: string[];
}

function roleLabel(r: string | CasdoorRole): string {
  if (typeof r === "string") return r;
  return r.displayName || r.name || "";
}

export class WhoamiCommand extends Command {
  static paths = [["whoami"]];
  static usage = Command.Usage({
    description: "显示当前 profile、server URL、用户信息",
  });

  async execute(): Promise<number> {
    const cfg = await loadConfig();
    const profileName = cfg.currentProfile;
    const profile = cfg.profiles[profileName];
    if (!profile) {
      process.stderr.write("未登录\n");
      return 3;
    }
    const tokenMask = profile.token.slice(0, 12) + "...";
    console.log(`Profile:    ${profileName}`);
    console.log(`Server:     ${profile.serverUrl}`);
    console.log(`Token:      ${tokenMask}`);

    const info = await apiRequest<UserInfo>("/auth/userinfo");
    const name = info.display_name || info.username || info.id;
    const roles = (info.roles ?? []).map(roleLabel).filter(Boolean);
    const rolesStr = roles.length ? roles.join(",") : "(无角色)";
    console.log(`User:       ${name} (${rolesStr})`);
    return 0;
  }
}
