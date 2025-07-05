#!/usr/bin/env -S mcphost script
---


---
Please list the files contained in the working directory ${env://WORK_DIR:-./}.
Order the files decendently by size. Show maximum ${count:-10} files.
Save the result in a text file in this local directory ./files.txt.
Make sure the JSON messages for the MCP tools are formatted correctly.