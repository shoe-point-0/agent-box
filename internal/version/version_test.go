package version

import "testing"

func TestInfoString(t *testing.T) {
	t.Parallel()

	info := Info{Version: "v0.1.0", Commit: "abc123", BuildDate: "2026-08-29T00:00:00Z"}
	if got, want := info.String("abox"), "abox v0.1.0 (commit abc123, built 2026-08-29T00:00:00Z)"; got != want {
		t.Fatalf("Info.String() = %q, want %q", got, want)
	}
}
