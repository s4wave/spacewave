# BlackShield Security Audit Report — `s4wave/spacewave`

- **Scanned Commit:** `e7f80ffd8631070e015db9238faa3acba824f9c5`
- **Actionable Findings:** `3` (`1 Critical`, `2 High`, `140 Medium`, `1 Low`)
- **BlackShield Platform & Continuous Zero-Cost PR Protection:** [https://blackshield.chaplau.com](https://blackshield.chaplau.com)

## Detailed Findings & Remediation Guide

### 1. [CRITICAL] `openssh-private-key` — OpenSSH Format Private Key Detected

- **Category:** `secret`
- **Location:** `app/device/AddDeviceWizardViewer.tsx:922`
- **Details:** Potential hardcoded credential or secret detected in `app/device/AddDeviceWizardViewer.tsx:922` (rule `openssh-private-key`). Raw value redacted (`***REDACTED***`) for safe disclosure.
- **Recommended Fix:** Revoke leaked credential immediately and rotate in secret manager.

### 2. [HIGH] `CKV_DOCKER_11` — CKV_DOCKER_11: Unverified Script Piped Directly to Shell

- **Category:** `misconfiguration`
- **Location:** `desktop/Dockerfile:42`
- **Details:** Piping remote HTTP downloads directly into sh or bash executes unverified code without checksum verification.
- **Recommended Fix:** Download scripts first, verify their SHA-256 checksum against an expected digest, then execute.

### 3. [HIGH] `CKV_DOCKER_1` — CKV_DOCKER_1: Container Runs as Root (Missing Non-Root USER)

- **Category:** `misconfiguration`
- **Location:** `desktop/Dockerfile:1`
- **Details:** Container does not specify a non-root USER, allowing processes to execute with root privileges.
- **Recommended Fix:** Add 'USER <non-root-uid>' instruction before container entrypoint.

