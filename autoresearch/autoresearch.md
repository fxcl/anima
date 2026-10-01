# Autoresearch: gola-vs-java-anima-alignment

## Objective

量化 Go 移植版 (本仓库) 相对 Java 参考实现 (com.hellokaton.anima v0.3.1,
位于 ~/go/anima/anima) 的 API 对齐程度, 并通过补齐缺口持续提高对齐率。

## Primary Metric

- `alignment_ratio` (higher is better): (covered + 0.5*partial) / total
  - total: manifest.tsv 中计分的 Java API 语义项 (当前 116)
  - covered=1, partial=0.5, gap=0
  - excluded 行 (sql2o 基础设施、内部枚举) 不计分

## Secondary Metrics (guardrails)

- `java_api_total`: 计分清单规模
- `covered` / `partial` / `gap`: 各状态计数
- `manifest_errors`: 双向断言失败数 (必须为 0, 否则脚本 exit 1)

## Benchmark Command

`./autoresearch/autoresearch.sh`

确定性双向断言:
- covered/partial 行: 每个 go 令牌必须出现在 `go doc -all .` / `go doc -all ./dialect`
  (src: 前缀则在 Go 源码中 grep)
- gap 行: 令牌必须不存在 — 若出现即 STALE-GAP (清单过时, 需把该行升级为 covered)

## Checks Command

`autoresearch/autoresearch.checks.sh` — go test 跑 TestAlign / TestGap /
TestErrorCode / TestAtomicNilDB 行为对齐守卫, 失败则 keep 无效。

## Files in Scope

- 源码: *.go (根包), dialect/*.go
- 清单: autoresearch/manifest.tsv (状态变更需同步更新)
- Java 参考 (只读): ~/go/anima/anima/src/main/java/com/hellokaton/anima/

## Off-limits

- java_align_test.go / contract_gap_test.go 的既有断言语义 (只能收紧不能放松)
- manifest.tsv 中 covered 行的 go 令牌 (除非 Java 侧对应物同步消失)

## Constraints

- Java 路径不可用时脚本 exit 2 (JAVA_SRC 环境变量可覆盖)
- 不得为凑分把 gap 行改成 covered 而不实现代码 — gap 行的 STALE-GAP 绊线保证实现先行

## What Has Been Tried (Segments 0-1)

- Segment 0 基线: alignment_ratio=0.8793 (covered=101, partial=2, gap=13, manifest_errors=0)
- Batch 1 (b7f5f62): AsBigInteger / Cond.NotEmpty / Anima 4 getter → 0.9310
- Batch 2+3a (20632af): AtomicOK/AtomicError/Rollback / Map / Filter → 0.9741
- Batch 3b (a7663d8): English(EnglishMode) 双模式 + 导出 Plural / ToUnderline / ToCamelName → 0.9914 (gap=0)
- Batch 4 (c8af182): Query.Seq (Java stream() → eager All + iter.Seq) / PluralCount
  (Java plural(word,count) 重载) → 1.0000 (116 covered + 0 partial + 0 gap)
- 计分口径: go doc 不显示未导出类型的导出方法 (如 *inflector.Plural)，
  此类行用 src: 令牌通道断言
- Java Stream parallel() (ForkJoin 运行时机制) 判定 Go 无惯用对应，不实现、不扣分
  (C10 锚点只锚 stream())

## Next (Segment 2 候选)

- 清单完备性审计: 二次提取 Java v0.3.1 全量 public 面，
  与 manifest 锚点 diff，确认 116 项无遗漏（发现即补 gap 行，如实降分再关闭）
