# Global Configuration System

Searchit provides a unified global configuration system that reuses the exact same overlay architecture as profiles. A configuration file is simply a persistent overlay that is automatically applied before user-selected profiles.

---

## Single Overlay Resolution Pipeline

Searchit enforces a single, deterministic 4-step resolution pipeline:

```
Built-in Defaults  ➜  User Config Overlay  ➜  Named Profile Overlays  ➜  CLI Flag Overrides
    (Step 1)                 (Step 2)                    (Step 3)                (Step 4)
```

1. **Built-in Defaults**: Hardcoded fallback values initialized via `config.Default()`.
2. **User Config Overlay**: Persistent overlays loaded from `config.yaml` (`scan:` or `fuzz:` section).
3. **Named Profile Overlays**: Explicit profile overlays specified via `-p/--profile` (applied in left-to-right order).
4. **CLI Flag Overrides**: Command-line flag options passed directly by the user.

---

## Configuration File Locations

Searchit searches for a global configuration file in the following order:

1. **XDG Base Directory Specification**: `$XDG_CONFIG_HOME/searchit/config.yaml`
2. **Standard User Home Fallback**: `~/.config/searchit/config.yaml`
3. **Explicit CLI Flag**: Overridden via `--config /path/to/config.yaml` or `-c /path/to/config.yaml`

If no configuration file exists at default locations, Searchit continues cleanly with built-in defaults without emitting warnings or errors. If an explicit `--config` path is specified but does not exist, Searchit halts with a configuration error.

---

## Configuration Format (`config.yaml`)

`config.yaml` uses tool-specific overlay sections (`scan` and `fuzz`) that match the exact schema used by profiles:

```yaml
# Persistent overlays for scan command
scan:
  threads: 64
  strategy: priority
  adaptive: true
  delay: 10ms
  timeout: 15s
  follow-redirects: true

# Persistent overlays for fuzz command
fuzz:
  threads: 64
  strategy: priority
  timeout: 15s
```

---

## Design Principles

- **Unified Schema**: No separate `defaults:` or `http:` schemas. `config.yaml` uses ordinary tool overlays (`scan.Overlay`, `fuzz.Overlay`).
- **Unified Merge Pipeline**: Uses the exact same `Apply` functions (`ApplyScanOverlay`, `ApplyFuzzOverlay`) as named profiles.
- **Strict Key Validation**: Unknown keys in `config.yaml` trigger validation errors on startup.
