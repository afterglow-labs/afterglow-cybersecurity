# ScopeWeaver v0.8.1 — Asset-rule and goal-proof integrity fixes

## Fixed

- An `exact_domain` allow rule now matches only the asset's actual host: its domain, bound domains or URL host ([#63](https://github.com/cskwork/scopeweaver/issues/63)). Stored root-domain metadata no longer satisfies the rule. For `{domain: "child.example.com", root_domain: "example.com"}`, allowing only `example.com` now rejects the child; explicitly allowing `child.example.com` accepts it. Block precedence and fuzzy-domain and root-only matching are unchanged. See [#68](https://github.com/cskwork/scopeweaver/pull/68).
- **Behavior change: `insert_assets` now fails closed when rule lookup fails** ([#64](https://github.com/cskwork/scopeweaver/issues/64)). Previously, a failed lookup was ignored and assets could be stored without the allowlist. Now, if either the global or the task interception-rule lookup fails, the tool returns an error before processing any asset. The whole batch is aborted, with no asset or automatic task-scope writes. Callers that previously continued during rule-storage failures must retry after rule lookup recovers. See [#68](https://github.com/cskwork/scopeweaver/pull/68).
- `prove_goal` now stores its evidence edge and goal state in one transaction ([#65](https://github.com/cskwork/scopeweaver/issues/65)). Any INSERT, UPDATE or COMMIT failure returns an error, rolls back both writes and does not arm goal completion. Previously, a goal could be proven without stored evidence. See [#68](https://github.com/cskwork/scopeweaver/pull/68).

## Changed

- Clarify the scope wording in the `add_task_scope` prompt, its five translations, the upstream changelog and schema comments ([#50](https://github.com/cskwork/scopeweaver/issues/50)). Scope declares the intended engagement targets and guides agent behavior and asset-query filtering; coverage uses it as a metric denominator. It does not technically restrict tool network access. To block out-of-scope tool calls, configure matching Interception deny rules; network isolation requires egress control outside ScopeWeaver. This clarification changes no scope or tool logic; execution-scope redesign remains open in #50.

## Known security issue

The Zod advisory disclosed in v0.8.0 is unchanged and still tracked in [issue #67](https://github.com/cskwork/scopeweaver/issues/67). See the [v0.8.0 note](../log/changelog-v0.8.0.md#known-security-issue).

## Upgrade and verification

v0.8.1 is a patch release with no new database migration. Back up the PostgreSQL database and data directory before upgrading, and stop the previous server before replacing it.

Local release-preparation verification passed `go build ./...`, the full `go test ./... -count=1 -json` with `ARTEX_TEST_DSN` set to a disposable native PostgreSQL database, and the separate `third_party/norma` Go suite. All 22 tested packages passed: 1,174 top-level tests passed, 4 skipped and none failed. The skips were the agent-adapter and chat-browser opt-ins, an unavailable Python interpreter and the private live-review configuration; DB-backed suites ran. Required hosted CI, browser E2E, release packaging and publication remain separate delivery gates.

## 한국어

### 수정

- `exact_domain` 허용 규칙은 이제 자산의 실제 호스트, 즉 도메인·연결된 도메인·URL 호스트와만 일치합니다([#63](https://github.com/cskwork/scopeweaver/issues/63)). 저장된 루트 도메인 메타데이터로는 규칙을 통과할 수 없습니다. 예를 들어 `{domain: "child.example.com", root_domain: "example.com"}`은 `example.com`만 허용하면 거부되고, `child.example.com`을 명시적으로 허용하면 통과합니다. 차단 규칙 우선순위, `fuzzy_domain`과 루트 도메인 전용 일치는 바뀌지 않습니다. [#68](https://github.com/cskwork/scopeweaver/pull/68)을 참고하세요.
- **동작 변경: 규칙 조회에 실패하면 `insert_assets`가 이제 거부합니다(fail closed)** ([#64](https://github.com/cskwork/scopeweaver/issues/64)). 이전에는 조회 실패를 무시해 허용 목록 없이 자산이 저장될 수 있었습니다. 이제 전역 또는 작업 가로채기(Interception) 규칙 조회 중 하나라도 실패하면 자산을 처리하기 전에 도구 오류를 반환합니다. 배치 전체가 중단되며 자산과 자동 작업 범위를 하나도 쓰지 않습니다. 규칙 저장소 오류 중에도 계속 진행하던 호출자는 규칙 조회가 복구된 뒤 다시 시도해야 합니다. [#68](https://github.com/cskwork/scopeweaver/pull/68)을 참고하세요.
- `prove_goal`은 이제 증거 연결과 목표 상태를 한 트랜잭션으로 저장합니다([#65](https://github.com/cskwork/scopeweaver/issues/65)). INSERT·UPDATE·COMMIT 중 하나라도 실패하면 오류를 반환하고 두 쓰기를 모두 되돌리며 목표 완료 상태를 설정하지 않습니다. 이전에는 증거가 저장되지 않은 채 목표가 증명될 수 있었습니다. [#68](https://github.com/cskwork/scopeweaver/pull/68)을 참고하세요.

### 변경

- `add_task_scope` 프롬프트와 5개 번역, 업스트림 변경 기록, 스키마 주석의 범위 표현을 명확히 했습니다([#50](https://github.com/cskwork/scopeweaver/issues/50)). 범위는 의도한 점검 대상을 선언하고 에이전트 행동과 자산 조회 필터링을 안내하며, 커버리지는 이를 지표 분모로 사용합니다. 범위 자체는 도구의 네트워크 접근을 기술적으로 제한하지 않습니다. 범위 밖 도구 호출을 차단하려면 일치하는 Interception deny 규칙을 설정하세요. 네트워크 격리에는 ScopeWeaver 바깥의 송신 트래픽 통제가 필요합니다. 이번 설명 수정은 범위·도구 로직을 바꾸지 않으며, 실행 범위 재설계는 #50에서 계속 다룹니다.

### 알려진 보안 이슈

v0.8.0에서 공개한 Zod 권고는 그대로이며 [이슈 #67](https://github.com/cskwork/scopeweaver/issues/67)에서 계속 추적합니다. [v0.8.0 안내](../log/changelog-v0.8.0.md#known-security-issue)를 참고하세요.

### 업데이트와 검증

v0.8.1은 새 데이터베이스 마이그레이션이 없는 패치 릴리스입니다. 업데이트 전에 PostgreSQL 데이터베이스와 데이터 디렉터리를 백업하고, 교체 전에 이전 서버를 중지하세요.

로컬 릴리스 준비 검증에서 `go build ./...`, `ARTEX_TEST_DSN`을 일회용 native PostgreSQL 데이터베이스로 설정한 전체 `go test ./... -count=1 -json`, 별도 `third_party/norma` Go 테스트를 통과했습니다. 테스트가 있는 22개 패키지가 모두 통과했으며, 최상위 테스트는 1,174개 통과·4개 건너뜀·실패 0개였습니다. 건너뛴 항목은 에이전트 어댑터·채팅 브라우저 선택 실행, Python 인터프리터 미설치, 비공개 실시간 검토 설정이며 DB 기반 테스트는 실행했습니다. 필수 원격 CI, 브라우저 E2E, 릴리스 패키징과 게시는 별도 전달 단계입니다.

[Compare v0.8.0...v0.8.1](https://github.com/cskwork/scopeweaver/compare/v0.8.0...v0.8.1)
