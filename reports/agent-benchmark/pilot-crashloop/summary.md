# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| catalog-crashloop | confined | 1 | 0 | 73016 | 20 | 0 | 1 |
| catalog-crashloop | unconfined | 1 | 1 | 95381 | 19 | 0 | 0 |

Episodes:
- catalog-crashloop r1: complete incident=2539c3c17daa155d41f3200b region_recall=false
