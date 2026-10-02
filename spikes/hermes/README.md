# Hermes spike (fake data)

A throwaway harness that answers one question before the Agent API is designed: how does
Hermes behave as the owner-facing assistant when every number must come from AI-BCC?

Nothing here touches a real shop. `mock_api.py` is a stand-in for the Agent API with two
invented shops and invented numbers. `aibcc_mcp.py` is the MCP shim Hermes calls. Findings are in
[FINDINGS.md](FINDINGS.md).

## Run it

Needs Docker, the Hermes image `nousresearch/hermes-agent:v2026.9.24` and an OpenRouter API key
(a normal key, not a management key, with a small credit limit).

```
./setup.sh                       # network, mock API, random per-shop tokens in tokens.env
./mkhome.sh                      # per-shop Hermes homes (config.yaml, SOUL.md)
printf 'OPENROUTER_API_KEY=%s\n' "<key>" > openrouter.env && chmod 600 openrouter.env
./ask.sh a "เดือนนี้ขายได้เท่าไหร่" <model>
./battery.sh <model>             # the 15-question set
```

`tokens.env`, `openrouter.env`, `data/`, `home-*/` and `runs/` are local and ignored by git.
Do not use a model that logs prompts (OpenRouter "stealth" models do) with real shop data.
