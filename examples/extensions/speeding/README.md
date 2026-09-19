# Speeding vertical example

This directory belongs to the Speeding consumer example, not to Agent Hub Core.
Nothing under this directory is compiled into or loaded automatically by the Hub.

`seed-agents.mjs` installs the example cast and directed relations into an already
running Hub. It is intentionally opt-in:

```bash
HUB_URL=http://127.0.0.1:8081 \
HUB_TOKEN=<admin-token> \
node examples/extensions/speeding/seed-agents.mjs
```

Set `DEPLOY=core` to deploy the three core example agents, or provide a
comma-separated list of example agent names. The script uses the tenant's
existing Aliyun Bailian provider credentials; it never accepts or stores an API
key itself.
