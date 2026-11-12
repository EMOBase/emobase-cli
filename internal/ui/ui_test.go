package ui

import (
	"strings"
	"testing"
)

func withColor(t *testing.T, enabled bool) {
	t.Helper()
	previous := color
	SetColor(enabled)
	t.Cleanup(func() { SetColor(previous) })
}

func TestHintKeepsBackticksWithoutColor(t *testing.T) {
	withColor(t, false)
	got := Hint("run `emobase doctor --fix` to apply safe fixes")
	want := "run `emobase doctor --fix` to apply safe fixes"
	if got != want {
		t.Fatalf("Hint() = %q, want %q", got, want)
	}
}

func TestHintKeepsCommandTextWithColor(t *testing.T) {
	withColor(t, true)
	got := Hint("run `emobase doctor --fix` to apply safe fixes")
	for _, want := range []string{"run ", "to apply safe fixes", "`emobase doctor --fix`"} {
		if !strings.Contains(got, want) {
			t.Errorf("Hint() = %q, missing %q", got, want)
		}
	}
}

func TestHintWithoutCommands(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		withColor(t, enabled)
		text := "server-side sign-in will fail; align it with KEYCLOAK_REALM"
		if got := Hint(text); !strings.Contains(got, text) {
			t.Errorf("color=%v: Hint() = %q, missing original text", enabled, got)
		}
	}
}

func TestHintMultipleCommands(t *testing.T) {
	withColor(t, false)
	text := "`cd /srv/emobase` and run `./init.sh`"
	if got := Hint(text); got != text {
		t.Fatalf("Hint() = %q, want %q", got, text)
	}
}
