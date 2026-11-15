# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| postgres-outage | confined | 1 | 0 | 141032 | 16 | 0 | 0 |
| postgres-outage | unconfined | 1 | 0 | 143863 | 19 | 0 | 0 |

Episodes:
- postgres-outage r1: complete incident=8f9e6a9f3b643de9e0f9cfae region_recall=false
