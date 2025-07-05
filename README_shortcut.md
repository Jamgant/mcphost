# Using MCP host with Local Ollama model

## Test connection to Ollama model
```bash
mcphost -m ollama:qwen2.5:7b -p "Explain quantum computing" --quiet
```

# Script with variables
```bash
mcphost script ./examples/scripts/first-mcp-server.sh --args:directory /tmp --args:name "Juan"
```