#!/usr/bin/env -S mcphost script
---
mcpServers:
  
  filesystem:
    type: "local"
    command: ["npx", "-y", "@modelcontextprotocol/server-filesystem", "./"]
    excludedTools: ["delete_file"]

model: "${env://MODEL:-ollama:qwen2.5:32b}"
---
Please list the files contained in the working directory ${env://WORK_DIR:-./}.
Order the files decendently by size. Show maximum ${count:-10} files.