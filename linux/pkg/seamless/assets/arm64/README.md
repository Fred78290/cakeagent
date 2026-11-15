Static `caker-seamlessd` for linux/arm64 goes here as `caker-seamlessd` (built by `scripts/build-seamlessd.sh`),
and is embedded into the agent. Without it the agent builds fine and `cakeagent seamless install` falls back to
`$CAKER_SEAMLESSD` or a binary on PATH.
