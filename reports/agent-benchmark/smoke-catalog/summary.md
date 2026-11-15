# Agent Benchmark

Model: qwen3.5:4b

| scenario | arm | runs | correct | tokens | tool calls | denials | expansions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| catalog-http-500 | confined | 1 | 1 | 110455 | 9 | 0 | 0 |
| catalog-http-500 | unconfined | 1 | 1 | 77554 | 12 | 0 | 0 |

Episodes:
- catalog-http-500 r1: complete incident=367dbacc37ea93078eda23c9 region_recall=true
