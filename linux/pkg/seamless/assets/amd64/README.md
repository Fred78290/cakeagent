Static `caker-seamlessd` for linux/amd64 goes here as `caker-seamlessd` (built by `scripts/build-seamlessd.sh`),
and is embedded into the agent, with the XKB data it needs next to it as `xkb/` (both come from the
`seamlessd` workflow's artifact). Without it the agent builds fine and `cakeagent seamless install` falls back to
`$CAKER_SEAMLESSD` or a binary on PATH.
