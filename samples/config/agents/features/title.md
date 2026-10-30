---
name: Title
description: Generate concise session titles from the conversation.
model:
  provider: ollama
  name: qwen3.5:9b-q8_0
  think: false
  context: 32768
tools:
  disabled:
    - '*'
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
