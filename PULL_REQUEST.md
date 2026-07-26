# Pull Request Guidelines

Thank you for contributing to Searchit. 

To maintain our engineering philosophy (Correctness > Determinism > Simplicity > Performance > Features), all Pull Requests must strictly adhere to the following conventions.

## 1. Pull Request Title

Your PR title must be a valid Searchit commit summary. It must start with a recognized prefix tag and an imperative, capitalized verb. Do not use trailing punctuation.

**Format:** `[PREFIX] Imperative action summary`

**Allowed Prefixes:**
- `[ADD]` - New features, tests, files, or capabilities.
- `[REFINE]` - Improvements, refactoring, or optimizations of existing code.
- `[FIX]` - Defect resolutions and correctness patches.
- `[UPDATE]` - Dependency bumps, version changes, or tooling updates.
- `[REMOVE]` - Deleting deprecated features or dead code.

**Examples:**
- ✅ `[ADD] Implement HTTP/2 check test`
- ✅ `[FIX] Correct wildcard detection return value`
- ❌ `Added new HTTP/2 tests` *(Wrong tense, missing prefix)*
- ❌ `[fix] wildcard detection` *(Lowercase prefix, missing verb)*

## 2. Pull Request Body

Use the standard template for your PR description. Keep it concise, objective, and evidence-driven.

### Summary
A concise explanation of the motivation or the problem being solved.

### Changes
- Past-tense bullet points describing the technical modifications.
- E.g., Added a test for HTTP/2 server configuration.
- E.g., Changed minimum Go version from 1.22+ to 1.23+.

### Validation
- Evidence proving the correctness and determinism of the changes.
- E.g., `make test` and `make chaos` passed successfully.
- E.g., Verified output idempotency across 5 consecutive runs.

## 3. Reviewer Expectations

- **Simplicity First:** Reviewers will evaluate if the proposed architecture is the simplest one capable of satisfying the requirements. Premature optimizations will be rejected.
- **Evidence-Driven:** Assertions of performance or correctness must be backed by validation output.
- **Editorial Voice:** PR bodies that are overly verbose, speculate, or use subjective, non-technical language will be requested to be revised to match Searchit's objective tone.
