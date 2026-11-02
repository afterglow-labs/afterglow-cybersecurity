# ScopeWeaver v0.8.0 — Six-language localization and opt-in OpenCode SDK bridge

## Added

- ScopeWeaver is now fully localized in English, Korean, Japanese, Spanish, Simplified Chinese (`zh-Hans`) and Traditional Chinese (`zh-Hant`). This covers the application UI, built-in prompts and report text, installer, launcher and adapter messages, user and contributor guides, and landing pages. English defaults, raw evidence and machine identifiers are unchanged. See [#52](https://github.com/cskwork/scopeweaver/pull/52).
- Add an opt-in `opencode-sdk` provider. It runs through a loopback-only bridge for native OpenCode CLI/client/SDK 2.1.4 (the default) or the legacy CLI / `@opencode-ai/sdk/v2` 1.19.4 pair (`SCOPEWEAVER_OPENCODE_RUNTIME=legacy`). The bridge streams one model step and captures tool requests. ScopeWeaver keeps conversation state, tool execution, host tool approval and scope checks, and OpenCode's built-in tools are refused before execution. Existing API and OpenCode Go profiles do not need the bridge. The release builder now includes the bridge source under `adapters/opencode-sdk`. See [#57](https://github.com/cskwork/scopeweaver/pull/57) and the [bridge guide](../adapters/opencode-sdk/README.md).

## Fixed

- Reconcile the agent and runtime contracts from #40 with current `dev`: terminal receipts, originating tool contexts, profile context windows, session ownership and server-wide shutdown drain. Default planner and worker prompts are now complete in Traditional Chinese, Japanese and Spanish. Saved custom prompts and history are unchanged. See [#60](https://github.com/cskwork/scopeweaver/pull/60).
- The legacy SDK runtime now rejects unmanaged system or MDM OpenCode configuration before launching the CLI, matching native mode. The checked paths are `/Library/Application Support/opencode` and `ai.opencode.managed.plist` under `/Library/Managed Preferences` on macOS, and `/etc/opencode` on Linux. The bridge fails with `configuration_error` (HTTP 400) and never reads or changes these sources. See [#66](https://github.com/cskwork/scopeweaver/pull/66).
- `fuzzy_domain` now matches whole domain labels ([#50](https://github.com/cskwork/scopeweaver/issues/50)). Block rules match labels anywhere: `.gov` matches `x.gov` and `x.gov.uk` but not `a.government.com`. Allow rules are anchored to the domain and its subdomains: `juice-shop.internal` allows `a.juice-shop.internal` but not `juice-shop.internal2` or `juice-shop.internal.attacker.com`. **Existing rules may now reject asset inserts that previously passed.** Agent `add_task_scope` and automatic worker additions must pass the current asset gate and fail closed if rules cannot be read. Manual scope edits and existing scope rows are unchanged.

Scope is prompt guidance plus a filter for asset recording and coverage. It is not a network boundary. To block tool calls, use Interception rules. A real network boundary requires egress control outside ScopeWeaver. `CHANGELOG.md` remains the [preserved upstream ARTEX history](../CHANGELOG.md). Its only addition since v0.7.0 is the #50 section.

## Known security issue

**HIGH: [GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5), Zod memory-exhaustion denial of service.** The OpenCode SDK adapter resolves `zod` **4.1.8** and, through `gitlab-ai-provider`, **3.25.76**. Both are below the fixed version **4.6.6**; Zod 3.x has no patch. The disclosed impact is **availability only (out-of-memory denial of service)** when oversized input reaches a size-constrained schema, including input from a malicious or compromised model provider or MCP server.

A full fix is upstream-gated: every reviewed `gitlab-ai-provider` release, from 6.19.0 through the reviewed latest 6.23.1, requires `zod ^3.25.76`. A global Zod 4 override breaks that dependency's 3.x contract. The release owner has accepted **disclose and track** for v0.8.0, without a code change; remediation is tracked in **[issue #67](https://github.com/cskwork/scopeweaver/issues/67)**. The GitLab provider path's reachability in ScopeWeaver is unverified and is not treated as a mitigation.

The recorded SDK dependency audit remains **exit 1**, separate from the recorded zero-finding web and agent-adapter audits. No new dependency audit was run for this publication.

## Upgrade and verification

Back up the PostgreSQL database and data directory before upgrading, and stop the previous server before replacing it. This release adds `opencode-sdk` to the profile-format database `CHECK` constraint. **Older binaries cannot start while SDK profiles remain.** Before downgrading, back up the database, then use v0.8.0 to convert SDK profiles to an older supported format or remove them. Migration was verified only on disposable databases, not on a live database.

Limitations:

- Both SDK modes are rejected on Windows.
- Native hosted free-model inference was verified on 2026-10-25 (KST), on macOS arm64 only. Free models, eligibility and quota can change, and lasting availability is not promised. Legacy hosted free access is unverified.

All seven required hosted checks passed on both integrated `dev` commit `2f071c3` and promoted `main` commit `cd23f63`: branch policy, frontend, agent adapter, backend, Windows, chat browser and smoke. These hosted results do not establish release-asset packaging or publication. Local runtime tests were not rerun for this publication.

## 한국어

### 추가

- 영어·한국어·일본어·스페인어·중국어 간체(`zh-Hans`)·중국어 번체(`zh-Hant`)를 완전히 지원합니다. 앱 화면, 기본 프롬프트와 보고서 문구, 설치·실행 프로그램과 어댑터 메시지, 사용자·기여자 안내서, 랜딩 페이지가 대상입니다. 영어 기본값, 원본 증거, 기계 식별자는 바뀌지 않습니다. [#52](https://github.com/cskwork/scopeweaver/pull/52)를 참고하세요.
- 선택형 `opencode-sdk` 제공자를 추가했습니다. 루프백 전용 브리지를 통해 native OpenCode CLI/client/SDK 2.1.4(기본값) 또는 legacy CLI / `@opencode-ai/sdk/v2` 1.19.4 조합(`SCOPEWEAVER_OPENCODE_RUNTIME=legacy`)을 사용합니다. 브리지는 모델 응답 한 단계를 스트리밍하고 도구 요청을 가로채기만 합니다. 대화 상태, 도구 실행, 호스트 도구 승인, 범위 검사는 ScopeWeaver가 그대로 맡고, OpenCode 내장 도구는 실행 전에 거부됩니다. 기존 API·OpenCode Go 프로필에는 브리지가 필요 없습니다. 릴리스 빌더가 압축 파일의 `adapters/opencode-sdk`에 브리지 소스를 포함하도록 수정했습니다. [#57](https://github.com/cskwork/scopeweaver/pull/57)과 [브리지 안내](../adapters/opencode-sdk/README.ko.md)를 참고하세요.

### 수정

- #40의 에이전트·런타임 계약을 현재 `dev`와 맞췄습니다. 대상은 종료 영수증, 원래 도구 문맥, 프로필 문맥 창, 세션 소유권, 서버 종료 시 작업 대기입니다. 중국어 번체·일본어·스페인어 기본 계획자·작업자 프롬프트도 완성했습니다. 저장된 사용자 프롬프트와 기록은 바뀌지 않습니다. [#60](https://github.com/cskwork/scopeweaver/pull/60)을 참고하세요.
- 이제 legacy SDK 런타임도 native처럼 CLI 실행 전에 관리되지 않는 시스템·MDM OpenCode 설정을 거부합니다. 확인하는 경로는 macOS의 `/Library/Application Support/opencode`와 `/Library/Managed Preferences` 아래 `ai.opencode.managed.plist`, Linux의 `/etc/opencode`입니다. 해당 설정이 있으면 `configuration_error`(HTTP 400)로 실패하며, 이 경로를 읽거나 바꾸지 않습니다. [#66](https://github.com/cskwork/scopeweaver/pull/66)을 참고하세요.
- `fuzzy_domain`은 이제 도메인 레이블 단위로 일치합니다([#50](https://github.com/cskwork/scopeweaver/issues/50)). 차단 규칙은 위치와 상관없이 레이블과 일치합니다. 예를 들어 `.gov`는 `x.gov`, `x.gov.uk`와 일치하지만 `a.government.com`과는 일치하지 않습니다. 허용 규칙은 해당 도메인과 하위 도메인에만 적용됩니다. 예를 들어 `juice-shop.internal`은 `a.juice-shop.internal`을 허용하지만 `juice-shop.internal2`나 `juice-shop.internal.attacker.com`은 허용하지 않습니다. **이전에 통과하던 자산 추가가 기존 규칙에 따라 거부될 수 있습니다.** 에이전트의 `add_task_scope`와 작업자 자동 추가도 현재 자산 검사를 거치며, 규칙을 읽지 못하면 추가하지 않습니다. 수동 범위 편집과 기존 범위 행은 바뀌지 않습니다.

범위는 프롬프트 안내이자 자산 기록·커버리지 필터이며, 네트워크 경계가 아닙니다. 도구 호출을 막으려면 Interception 규칙을 쓰세요. 실제 네트워크 경계가 필요하면 ScopeWeaver 바깥에서 송신 트래픽을 통제해야 합니다. `CHANGELOG.md`는 [보존된 업스트림 ARTEX 기록](../CHANGELOG.ko.md)이며, v0.7.0 이후 추가된 내용은 #50 항목뿐입니다.

### 알려진 보안 이슈

**HIGH: [GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5), Zod 메모리 고갈 서비스 거부 취약점.** OpenCode SDK 어댑터는 `zod` **4.1.8**과 `gitlab-ai-provider`를 통한 **3.25.76**을 사용합니다. 둘 다 수정 버전 **4.6.6**보다 낮으며, Zod 3.x에는 패치가 없습니다. 공개된 영향은 **가용성에 한정된 메모리 고갈(OOM) 서비스 거부**입니다. 악성·침해된 모델 제공자나 MCP 서버 등에서 들어온 과도하게 큰 입력이 크기 제한 스키마에 도달할 때 발생할 수 있습니다.

완전한 수정은 업스트림 변경에 달려 있습니다. 검토한 `gitlab-ai-provider`의 모든 릴리스인 6.19.0부터 당시 최신 6.23.1까지 `zod ^3.25.76`을 요구하며, 전체를 Zod 4로 강제하면 해당 의존성의 3.x 계약이 깨집니다. 릴리스 책임자는 코드 변경 없이 v0.8.0에서 **공개 후 추적**하는 방식을 승인했습니다. 후속 수정은 **[이슈 #67](https://github.com/cskwork/scopeweaver/issues/67)**에서 추적합니다. ScopeWeaver에서 GitLab 제공자 경로가 실제 호출되는지는 검증되지 않았으며, 이를 완화책으로 간주하지 않습니다.

기록된 SDK 의존성 감사는 여전히 **종료 코드 1**입니다. 취약점 0건으로 기록된 웹·에이전트 어댑터 감사와는 별개이며, 이번 게시 준비에서 의존성 감사를 새로 실행하지 않았습니다.

### 업데이트와 검증

업데이트 전에 PostgreSQL 데이터베이스와 데이터 디렉터리를 백업하고, 교체 전에 이전 서버를 중지하세요. 이번 릴리스는 프로필 형식의 데이터베이스 `CHECK` 제약 조건에 `opencode-sdk`를 추가합니다. **SDK 프로필이 남아 있으면 이전 버전 실행 파일은 시작되지 않습니다.** 이전 버전으로 되돌리려면 먼저 데이터베이스를 백업하세요. 그런 다음 v0.8.0에서 SDK 프로필을 이전 버전이 지원하는 형식으로 바꾸거나 삭제하세요. 마이그레이션은 일회용 데이터베이스에서만 검증했으며, 운영 데이터베이스에서는 검증하지 않았습니다.

제한 사항:

- Windows에서는 두 SDK 모드를 모두 지원하지 않습니다.
- native 모드의 호스팅 무료 모델 추론은 2026-10-25(KST)에 macOS arm64에서 검증했습니다. 무료 모델, 사용 자격, 할당량은 바뀔 수 있으며 계속 사용할 수 있다고 보장하지 않습니다. legacy 모드의 호스팅 무료 사용은 검증하지 않았습니다.

통합 `dev` 커밋 `2f071c3`과 승격된 `main` 커밋 `cd23f63`에서 필수 원격 검사 7개인 브랜치 정책·프런트엔드·에이전트 어댑터·백엔드·Windows·채팅 브라우저·스모크가 모두 통과했습니다. 이 결과는 릴리스 파일의 패키징이나 게시까지 입증하지 않습니다. 이번 게시 준비에서 로컬 런타임 테스트를 다시 실행하지 않았습니다.

[Compare v0.7.0...v0.8.0](https://github.com/cskwork/scopeweaver/compare/v0.7.0...v0.8.0)
