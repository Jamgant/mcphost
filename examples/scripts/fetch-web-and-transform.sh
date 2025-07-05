#!/usr/bin/env -S mcphost script
---
mcpServers:
  
  filesystem:
    type: "local"
    command: ["npx", "-y", "@modelcontextprotocol/server-filesystem", "./"]
    excludedTools: ["delete_file"]
  
  web-fetcher: 
    type: "builtin"
    name: "fetch"

model: "${env://MODEL:-ollama:qwen2.5:32b}"
---
Fetch the following web page ${webpage:-https://ollama.com/library/mistral} and save only the title of the page in a text file in this local directory ./
Use the ${tool:-fetch} tool to fetch the web page.
Use the tool ${tool:-filesystem} to save the file.