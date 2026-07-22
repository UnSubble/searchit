[Index](../../README.md) | Keyboard Shortcuts

# Keyboard Shortcuts

This document provides a reference for the keyboard shortcuts available in the Searchit interactive TUI.

> **Note**: Keyboard input is only available when the progress display is active (not in `-q` / quiet mode, not in `--no-progress` mode). However, `Ctrl+C` always works regardless of progress mode.

| Shortcut | Description |
|:---|:---|
| `p` | pause / resume (scan and fuzz) |
| `s` | toggle statistics view (scan and fuzz) |
| `q` | stop current target (scan only; moves to next target if multi-target scan) |
| `Ctrl+C` | abort everything; waits for in-flight requests to complete (scan and fuzz) |

### Details

* **`p`**: Pauses or resumes the active execution for both scan and fuzz commands.
* **`s`**: Toggles the statistics view on and off to show advanced telemetry and engine status during execution (scan and fuzz).
* **`q`**: Stops the current target in a scan operation. If scanning multiple targets, it gracefully stops the current target and moves on to the next one. This feature applies to scan only.
* **`Ctrl+C`**: Aborts everything. Triggers a graceful shutdown, waiting for any in-flight requests to complete and ensuring clean resource teardown before exiting the program. Works in both scan and fuzz modes, and is always active regardless of whether the progress display is enabled.
