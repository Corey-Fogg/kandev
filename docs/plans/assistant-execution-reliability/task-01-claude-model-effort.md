---
id: "01-claude-model-effort"
title: "Allow Claude model and effort settings"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-004
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-004.1
  - AC-ORCHESTRATION-ASSISTANT-004.2
  - AC-ORCHESTRATION-ASSISTANT-004.5
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 01: Allow Claude model and effort settings

## Summary

Permit a restricted Claude assistant profile to use its configured Claude model
and provider-supported effort value. Preserve the managed Claude ACP version,
broker-only tools, local executor restrictions and all existing invocation-time
authority checks.

## Scope

- Replace the blanket rejection of profile config options with a narrow
  compatibility check for supported Claude `effort` options.
- Preserve model selection in the profile and include both model and effort in
  the authority fingerprint so a changed selection cannot resume stale state.
- Verify the values reach Claude ACP session configuration without changing the
  managed assistant policy metadata.
- Keep rejection of every other unsupported profile override actionable.

## Exclusions

Do not qualify other providers or Claude ACP versions. Do not enable arbitrary
config options, CLI flags, environment overrides, fallbacks, custom commands,
provider-native tools, external MCP attachments or executor setup scripts.

## Acceptance

1. A supported Claude model and each provider-supported effort value pass
   restricted-assistant compatibility and are applied to a newly created
   session.
2. Unsupported config-option IDs and all existing unsafe profile overrides
   remain rejected before process launch.
3. Model/effort changes alter the authority revision, while tool-policy options
   remain identical on create and resume.

## Likely files

- `apps/backend/internal/backendapp/assistant_authority.go`
- `apps/backend/internal/backendapp/assistant_authority_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/assistant_policy.go`
- Assistant profile/session preparation and its focused tests under
  `apps/backend/internal/backendapp/`

## Verification

```sh
(cd apps/backend && go test -count=1 ./internal/backendapp -run 'TestAssistant(Restriction|Authority|Launch)')
(cd apps/backend && go test -count=1 ./internal/agentctl/server/adapter/transport/acp -run 'Test.*Assistant')
```

## Dependencies and risks

No dependencies. Confirm from the pinned ACP implementation that model and
effort updates use the qualified session configuration path; do not infer
support solely from UI profile fields.

## Results

Pending implementation.
