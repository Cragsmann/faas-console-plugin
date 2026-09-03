# SRVOCF-1075: System Namespace Warning on Create Function

**Status:** ACTIVE

**Jira:** [SRVOCF-1075](https://redhat.atlassian.net/browse/SRVOCF-1075) (Bug, parent SRVOCF-953)

**Goal:** Warn (but do not block) when a user enters a system namespace on the Create Function form. System namespaces are not meant for user workloads and function deployment there fails or produces unexpected errors. Guide the user to create a dedicated namespace for functions.

## Background

- The Create Function form's Namespace field is a free-text `TextInput` (`src/pages/function-create/components/CreateFunctionForm.tsx:130-137`), not a dropdown seeded from a live Namespace object. Detection must therefore be name/prefix based, since only a string is available at typing time.
- No system-namespace detection exists anywhere in the codebase today (net-new logic).
- Existing inline warning-Alert pattern to match: `src/pages/function-create/FunctionCreatePage.tsx:42-50` (`Alert variant="warning" ... isInline`).
- Related: [SRVOCF-1080](https://redhat.atlassian.net/browse/SRVOCF-1080) (system namespace deployment doesn't work).

## System namespace definition

- Prefixes: `openshift-`, `kube-`
- Exact names: `default`, `openshift`, `kube-system`, `kube-public`, `kube-node-lease`

## Tasks

### Task 1: Add `isSystemNamespace` helper (TDD)

- Add pure helper `isSystemNamespace(name: string): boolean` to `src/common/utils/utils.ts`.
- Matches the prefix and exact-name lists above. Handles empty/whitespace input (returns `false`).
- Unit tests first: system prefixes, exact names, normal namespaces, empty string, case sensitivity (namespaces are lowercase, so exact match is fine).

### Task 2: Surface the warning on the Create Function form

- When `isSystemNamespace(fields.namespace)` is true, render an inline `Alert variant="warning" isInline` near the Namespace `FormGroup` (or in the page `PageSection`, matching the existing token warning).
- Message: explain functions should not run in system namespaces and advise creating a new namespace for functions.
- Do **not** disable Create / Deploy; submission still works.
- Use i18n `t(...)` per project convention.

### Task 3: Component tests

- Warning appears when a system namespace is typed; absent for a normal namespace.
- Submit button remains enabled with a system namespace entered.

## Acceptance criteria (from Jira)

- Inline warning alert appears near the field when the namespace matches a system-namespace pattern.
- Message advises creating a new namespace for functions.
- Create / Deploy is not disabled; submission still works.
- Non-system namespaces show no warning.
- Detection is a unit-tested pure helper (`isSystemNamespace`).
- Warning matches the existing inline warning-Alert pattern on the create page.

## Out of scope

- Blocking/blacklisting system namespaces (tracked separately; warn-only per this story).
- Fixing deployment to system namespaces (SRVOCF-1080).
