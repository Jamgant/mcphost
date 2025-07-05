#!/usr/bin/env -S mcphost script
---
mcpServers:
  
  filesystem:
    type: "local"
    command: ["npx", "-y", "@modelcontextprotocol/server-filesystem", "./"]
    excludedTools: ["delete_file"]
    environment:
      DEBUG: "true"
      LOG_LEVEL: "info"
  
  web-fetcher: 
    type: "builtin"
    name: "fetch"

model: "${env://MODEL:-ollama:qwen2.5:32b}"
---
Please list the files contained in the working directory ${env://WORK_DIR:-./}.
Order the files decendently by size. Show maximum ${count:-10} files.
Save the result in a text file in this local directory ./files.txt.