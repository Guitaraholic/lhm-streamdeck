# SparkDash LLM metrics — GitHub release notes for v2.2.0-macos.1

Adds support for [sparkDash](https://github.com/MiaAI-Lab/sparkDash) polling
for metrics on LLM model performance.

### What's changed

#### New features

- **SparkDash source profiles.** A Settings source can now be Kind
  **SparkDash** instead of Libre Hardware Monitor. Point it at the dashboard
  (port **5555** for SparkDash’s own HTTP listener, **443** for HTTPS) and pick
  a unit. Each unit is its own profile.
- **Live model-performance tiles.** Decode and prefill tok/s, KV cache and
  queue health appear in the sensor picker under the **LLM** category, so a key
  can watch how fast a model is generating rather than only GPU load.
- **Lab tiles for token rates.** Middle label is `tok/s` or `prefill/s`; the
  unit is not drawn next to the number (percent readings still show `%`).

Hardware companion tiles, macOS, and DGX Spark agent installs are unchanged.

### Downloads

| File | What it is |
|---|---|
| `com.moeilijk.lhm.streamDeckPlugin` | The plugin for macOS. Universal (arm64 + x86_64), with the macOS companion bundled |
| `install-lhm-companion.sh` | Self-extracting agent installer for Linux hosts. Carries **both** x86_64 and aarch64 binaries; needs no network access on the target |
