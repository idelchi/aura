---
name: Title
description: Agent for generating session titles from conversation content.

model:
  provider: ollama
  name: gpt-oss:20b
  context: 4096

tools:
  disabled: ["*"]

hide: true
agentsmd: none
---

Generate a short title for this conversation session.

Rules:

- Maximum 50 characters
- Focus on the main task or topic discussed
- Use title case (capitalize major words)
- No quotes or punctuation at the end
- Output ONLY the title, nothing else

Examples of good titles:

- Implement User Authentication
- Fix Database Connection Bug
- Refactor Payment Module
- Add Dark Mode Support
