# the horse branch

Our fork of neru, the keyboard mouse on bekh's desk. Upstream's `AGENTS.md` is still the guide to
the code; this file is what the fork changes, how bekh's setup is wired, and what bit us.

## Build and install

`./horse-build.sh` builds, copies the binary into `/Applications/Neru.app` (the cask's bundle) and
signs it with bekh's Apple Development cert, so the Accessibility grant survives rebuilds. It quits
and relaunches a running neru. `just`, `oku` and `golangci-lint` aren't installed here: run the
recipe steps by hand (`go test ./internal/...`, `go build ./...`), and pin lint to the version in
`oku.lock`. Two tests fail on a clean tree: `TestAdapter_RolePassing` (flaky) and
`TestLaunchCommandExecution` (fails while neru is running).

## What the fork changes (`// horse:` in the code)

- **Lowercase labels.** Hints and grid draw labels lowercase. Native matching is `hasPrefix`,
  which is case-sensitive, so the typed prefix is lowered too (`UpdateMatches`,
  `updateMatchesIncremental`). Lower one side without the other and live matching dies.
- **System prompts get hints.** Permission and system dialogs belong to background agents, never
  the frontmost app, so hints used to come up blank over them. An always-on supplementary source
  (`addSystemPromptElements`) sweeps `com.apple.accessibility.universalAccessAuthWarn`,
  `com.apple.UserNotificationCenter` and `com.apple.coreservices.uiagent`. A prompt that still
  shows no labels means one more bundle ID for that list: get it with `lsappinfo info -only
  bundleid <process>`.
- **Hints opens on an empty screen.** Zero hints still enters the mode, so the hooks fire, the
  glow shows, and escape or the toggle closes it.
- **Pass-through is always on.** In every mode, a key with cmd, alt or ctrl, or any F1–F20, that
  the mode doesn't bind passes to the system and closes the mode (alt-t reaches iterm, dictation's
  cmd-v pastes). Bare and shift-only keys are still swallowed. The decision is made in the native
  tap callback (`eventtap_darwin.m`). Upstream's `passthrough_unbounded_keys` and
  `should_exit_after_passthrough` do nothing in the fork. The tap ignores modifiers neru itself
  holds (sticky modifiers are on), or a label typed under sticky cmd would pass through and close
  the mode.

## bekh's setup

The config is `~/.config/neru/config.toml`, not in any repo; `neru config reload` applies it.

- **Keys (F18 is a Lily58 key):** `F18` hints, `shift-F18` right-click hints, `alt-F18` the letter
  grid, `ctrl-F18` vision hints. All of them carry `--toggle`, so pressing again closes the mode.
  The desk-wide list is `~/tower/functional/lily58-zmk/docs/chords.txt`; the neru line is a hand
  claim in `docs/chords.hand`.
- **Grid:** `max_label_length = 2`, font 14. bekh likes the dense two-letter grid, Scoot-style; the
  recursive grid was tried and dropped as slower.
- **In grid and hints:** Enter is the `click_and_exit` macro; arrows nudge 5 px in every mode.
- **The glow:** hooks `on_mode_enter`/`on_mode_exit` call `shit glow on|off`, which lights a purple
  vignette on every screen edge (hyprshit's `ModeFrame`; look in `hyprshit/config/neru`), so a mode
  shows even when its labels are behind a window.

## Gotchas

- **Vision without Screen Recording kills neru.** Neru has no Screen Recording grant yet; a vision
  pass asks TCC and the process dies quietly, a go death with no crash report (find it with
  `log show --predicate 'process == "neru"'`). It dies with a mode up, so the exit hook never fires
  and the glow sticks: `shit glow off`. Grant Neru in System Settings, Privacy, Screen Recording
  before relying on `ctrl-F18`.
- **`--toggle` refuses a `--strategy` list.** A key can't both toggle and cycle strategies.
- **Fake clicks do land on permission prompts.** Grid plus Enter (a synthetic click) dismissed the
  accessibility prompt; the problem there was never the click, only the missing labels. An AX press
  (`click button "Deny"` through System Events) works too.
- **The practice dummy:** `~/tower/ephemeral/tcc-dummy`, a signed app that pops a real prompt on
  demand. `./poke ax` or `./poke screen` resets its grant and pops the prompt again; `./make.sh`
  rebuilds it. Our own screenshot missed the prompt window once while AX saw it, so check with AX
  (`tell process "universalAccessAuthWarn" to get every window`), not by eye on a capture.
