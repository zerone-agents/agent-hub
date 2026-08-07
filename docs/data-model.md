# Data Model

```
tenant.Tenant (1) ─┬─ (N) tenant.User
                   ├─ (N) tenant.ServiceDeployment
                   └─ (N) tenant.Resource

agent.AgentConfig (1) ─┬─ (N) agent.AgentSubagent → AgentConfig
                       ├─ (N) agent.AgentTool     → agent.Tool
                       └─ (N) agent.AgentSkill    → skill.Skill

scene.Scene (N) ─→ (1) agent.AgentConfig

chat.Session (composite PK: user_id + id) ─┬─ (N) chat.Message (composite PK: user_id + session_id + id)
                                            └─ fields: model / agent_id / provider_id / mode / ...

provider.Provider (vendor_presets table, LockedAPIKey encrypted with AES-GCM)
```

AutoMigrate runs automatically on application startup (in production, consider extracting it into a separate Job to avoid multi-replica race conditions).
