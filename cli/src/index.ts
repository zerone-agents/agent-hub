#!/usr/bin/env bun
import { Cli, Builtins } from "clipanion";
import { version } from "../package.json";
import { LoginCommand } from "./commands/login";
import { WhoamiCommand } from "./commands/whoami";
import {
  AgentListCommand,
  AgentGetCommand,
  AgentCreateCommand,
  AgentUpdateCommand,
  AgentDeleteCommand,
  AgentSetSubagentsCommand,
  AgentSetToolsCommand,
  AgentSetMcpsCommand,
  AgentSetSkillsCommand,
  AgentDeployCommand,
  AgentUndeployCommand,
  AgentStartCommand,
  AgentStopCommand,
  AgentStatusCommand,
} from "./commands/agent";
import {
  ProviderListCommand,
  ProviderGetCommand,
  ProviderCreateCommand,
  ProviderUpdateCommand,
  ProviderDeleteCommand,
  ProviderProbeCommand,
} from "./commands/provider";
import {
  ToolListCommand,
  ToolGetCommand,
  ToolCreateCommand,
  ToolUpdateCommand,
  ToolDeleteCommand,
} from "./commands/tool";
import {
  McpListCommand,
  McpGetCommand,
  McpCreateCommand,
  McpUpdateCommand,
  McpDeleteCommand,
  McpProbeCommand,
} from "./commands/mcp";
import {
  SkillListCommand,
  SkillGetCommand,
  SkillCreateCommand,
  SkillUpdateCommand,
  SkillDeleteCommand,
  SkillDownloadCommand,
} from "./commands/skill";

const cli = new Cli({
  binaryName: "zhub",
  binaryVersion: version,
});

cli.register(LoginCommand);
cli.register(WhoamiCommand);
cli.register(Builtins.HelpCommand);
cli.register(Builtins.VersionCommand);

// agent
cli.register(AgentListCommand);
cli.register(AgentGetCommand);
cli.register(AgentCreateCommand);
cli.register(AgentUpdateCommand);
cli.register(AgentDeleteCommand);
cli.register(AgentSetSubagentsCommand);
cli.register(AgentSetToolsCommand);
cli.register(AgentSetMcpsCommand);
cli.register(AgentSetSkillsCommand);
cli.register(AgentDeployCommand);
cli.register(AgentUndeployCommand);
cli.register(AgentStartCommand);
cli.register(AgentStopCommand);
cli.register(AgentStatusCommand);

// provider
cli.register(ProviderListCommand);
cli.register(ProviderGetCommand);
cli.register(ProviderCreateCommand);
cli.register(ProviderUpdateCommand);
cli.register(ProviderDeleteCommand);
cli.register(ProviderProbeCommand);

// tool
cli.register(ToolListCommand);
cli.register(ToolGetCommand);
cli.register(ToolCreateCommand);
cli.register(ToolUpdateCommand);
cli.register(ToolDeleteCommand);

// mcp
cli.register(McpListCommand);
cli.register(McpGetCommand);
cli.register(McpCreateCommand);
cli.register(McpUpdateCommand);
cli.register(McpDeleteCommand);
cli.register(McpProbeCommand);

// skill
cli.register(SkillListCommand);
cli.register(SkillGetCommand);
cli.register(SkillCreateCommand);
cli.register(SkillUpdateCommand);
cli.register(SkillDeleteCommand);
cli.register(SkillDownloadCommand);

cli.runExit(process.argv.slice(2));
