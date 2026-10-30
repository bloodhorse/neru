package modes

import (
	"slices"
	"strings"

	"go.uber.org/zap"

	configpkg "github.com/y3owk1n/neru/internal/config"
	"github.com/y3owk1n/neru/internal/domain"
)

// syncModifierPassthrough tells the event tap which keys the active mode
// answers to, so it consumes those and passes the rest through.
//
// Both lists it builds are the keys of the keymap in force, read rather than
// merged: this runs on the triggers that settle one — a mode change, a
// configuration replacement, a hints refresh after the focused app changed, and
// a focused-app change under an open mode — so there is nothing here to
// invalidate. Every caller passes the mode that is active, which is what makes
// the settled keymap the right one to read.
//
// Caller must hold h.mu.
func (h *handlerState) syncModifierPassthrough(mode domain.Mode) {
	if !h.hasEventTap() {
		return
	}

	enabled := h.passthroughEnabledFor(mode)

	h.setPassthroughCallback(h.passthroughCallbackFor(mode, enabled))

	// The keymaps are only consulted when passthrough is on: with it off the tap
	// consumes everything anyway, and a mode that is not open binds nothing.
	keymap := configpkg.Keymap{}
	globalHotkeys := configpkg.Keymap{}
	blacklist := []string(nil)

	if enabled {
		keymap, globalHotkeys = h.settledKeymaps()

		blacklist = append(blacklist, h.config.General.PassthroughUnboundedKeysBlacklist...)

		// The keys the mode binds must also be blacklisted so the event tap
		// consumes them instead of passing them through. So must the global
		// chords the mode falls back to, and for the same reason: passed through,
		// the tap would hand the user's own hotkey to the application in front of
		// them and the fallback would never see the key at all.
		for _, key := range append(keymap.Keys(), globalHotkeys.Keys()...) {
			blacklist = append(blacklist, configpkg.CanonicalHotkeyForPlatform(key))
		}
	}

	h.setModifierPassthrough(enabled, blacklist)

	h.setInterceptedModifierKeys(modeModifierKeys(keymap, globalHotkeys))
}

// passthroughEnabledFor reports whether the event tap should hand unbound
// modifier chords to the focused application while mode is active. Idle counts
// as off: a mode that is not open binds nothing, so there would be nothing to
// hold back.
//
// Caller must hold h.mu.
//
// horse: on in every open mode whatever general.passthrough_unbounded_keys
// says; an unbound Cmd/Alt/Ctrl chord or F-key reaches the app and closes the
// mode (passthroughTick).
func (h *handlerState) passthroughEnabledFor(mode domain.Mode) bool {
	return h.config != nil && mode != domain.ModeIdle
}

// RefreshPassthroughForFocusedAppChange re-synchronizes the event tap with the
// bindings the application the user just switched to puts in force, so the keys
// the tap consumes and the keys the mode is bound to keep describing the same
// set while a mode stays open across an application switch.
//
// The keymap can settle lazily because a keystroke reads it, so the read is the
// trigger (ADR 0005). Passthrough cannot: the blacklist is what decides whether
// the next chord reaches Neru at all, so waiting for a keystroke means waiting
// for the keystroke that already went to the other application. This has to be
// pushed, and it is pushed from a goroutine the app layer starts
// (`handleAppActivation`, `app/lifecycle.go`) — never inline from the watcher
// callback, which on macOS runs on the main queue, where taking h.mu is
// forbidden (internal/app/modes/AGENTS.md). What the goroutine buys is only
// that the handler is reached off the main queue: this still queues behind
// whatever holds h.mu, so it lands as soon as the handler is free rather than
// in step with the focus change.
//
// It reads the published cell rather than taking the application as an
// argument, which is what makes two of these racing harmless: whichever runs
// last reads the newest publication and the other is a no-op against an
// unchanged keymap. Reading it is also all it will ever do: settling a keymap
// with nothing published asks the platform, and that call is the unbounded
// cross-process one ADR 0005 keeps off h.mu for everything the user is not
// waiting on. Nobody waits on a focus change, so this returns instead.
func (h *Handler) RefreshPassthroughForFocusedAppChange() {
	h.mu.Lock()
	defer h.mu.Unlock()

	mode := h.appState.CurrentMode()

	// Nothing to move unless all three hold: something has to have announced
	// the application, the tap has to be routing chords by the lists at all,
	// and the active mode has to bind something the focused application can
	// change — the last is the reasoning keymapInputs applies to the keymap.
	// With any of them missing, the state set when the mode opened still
	// describes what is bound.
	if _, published := h.focusedApp.published(); !published {
		return
	}

	// The last of the three asks about both tables in force, not just the mode's
	// own: a global chord the mode falls back to is on the blacklist too, and an
	// [[app_configs]] entry can rebind one, so the focused application moving can
	// move that half as well.
	if !h.passthroughEnabledFor(mode) || !h.focusedAppCanChangeWhatIsBound(mode) {
		return
	}

	h.syncModifierPassthrough(mode)
}

func (h *handlerState) passthroughCallbackFor(mode domain.Mode, enabled bool) func() {
	if !enabled {
		return nil
	}

	session := h.modeSession

	return func() {
		h.outer.handlePassthrough(mode, session)
	}
}

const initialCapacity = 16

// modeModifierKeys is the modifier chords the given keymaps bind, in the form
// the platform canonicalizes them to: the event tap intercepts these instead of
// passing them through to the focused application.
//
// It takes both tables in force while a mode is open — the mode's own, and the
// global chords it falls back to — because the tap asks a single question about a
// chord, so the two reach it as one list. A chord both tables bind is one entry,
// which is also the answer either way.
func modeModifierKeys(mode, global configpkg.Keymap) []string {
	keys := make([]string, 0, initialCapacity)
	seen := make(map[string]struct{}, initialCapacity)

	for _, keymap := range []configpkg.Keymap{mode, global} {
		for _, key := range keymap.Keys() {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" || !configpkg.HasPassthroughModifier(trimmed) {
				continue
			}

			normalized := configpkg.CanonicalHotkeyForPlatform(trimmed)
			if _, exists := seen[normalized]; exists {
				continue
			}

			seen[normalized] = struct{}{}

			keys = append(keys, normalized)
		}
	}

	if len(keys) == 0 {
		return nil
	}

	slices.Sort(keys)

	return keys
}

// handlePassthrough is called when a modifier shortcut was passed through to
// macOS while a mode was active. The callback carries the mode/session that
// were current when the event tap observed the passthrough so late callbacks
// cannot act on a different activation.
func (h *Handler) handlePassthrough(mode domain.Mode, session uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.passthroughTick(mode, session)
}

// passthroughTick is called when a key was passed through to the OS while a
// mode was active. The mode/session arguments identify the originating
// activation so stale callbacks can be ignored safely.
//
// horse: a pass always closes the mode, whatever
// general.should_exit_after_passthrough says; the hints refresh it replaced
// had nothing left to refresh.
//
// Caller must hold h.mu.
func (h *handlerState) passthroughTick(mode domain.Mode, session uint64) {
	if h.modeSession != session || h.appState.CurrentMode() != mode {
		return
	}

	h.cancelPendingModifierToggle()

	h.logger.Debug("Exiting mode after passthrough",
		zap.String("mode", domain.ModeString(mode)),
		zap.Uint64("session", session))
	h.exitMode()
}
