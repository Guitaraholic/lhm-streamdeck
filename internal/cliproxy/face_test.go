package cliproxy

import (
	"testing"
	"time"
)

func faceService(t *testing.T, quota *AccountQuota) *Service {
	t.Helper()
	svc := NewServiceWithClient(nil)
	svc.accounts["k1"] = Account{
		AuthIndex: flexString("k1"),
		Provider:  "claude",
		Email:     "kev@example.com",
		Status:    "ready",
	}
	if quota != nil {
		svc.quotas["k1"] = quota
	}
	return svc
}

func TestQuotaFaceCarriesAllWindows(t *testing.T) {
	reset := time.Now().Add(90 * time.Minute)
	far := time.Now().Add(4 * 24 * time.Hour)
	svc := faceService(t, &AccountQuota{
		FetchedAt: time.Now(),
		Primary:   &QuotaWindow{Label: "Session", UsedPercent: 42, ResetAt: &reset, WindowSeconds: 5 * 3600},
		Secondary: &QuotaWindow{Label: "Weekly", UsedPercent: 7, ResetAt: &far, WindowSeconds: 7 * 24 * 3600},
	})
	suid := "/cliproxy/account/k1"

	// Every used-slot resolves the same rotating face carrying both windows.
	for _, path := range []string{"/quota/session/used/0", "/quota/weekly/used/0"} {
		f, ok := svc.QuotaFace(suid, ReadingIDFor(suid, path))
		if !ok {
			t.Fatalf("%s should resolve to a face", path)
		}
		if f.Label != "CLAUDE" {
			t.Fatalf("label = %q", f.Label)
		}
		if len(f.Windows) != 2 {
			t.Fatalf("windows = %d, want session+weekly", len(f.Windows))
		}
		if f.Windows[0].Label != "Session" || f.Windows[0].UsedPercent != 42 {
			t.Fatalf("primary window = %+v", f.Windows[0])
		}
		if f.Windows[1].Label != "Weekly" || f.Windows[1].UsedPercent != 7 {
			t.Fatalf("secondary window = %+v", f.Windows[1])
		}
	}
}

func TestQuotaFaceSkipsMissingWindows(t *testing.T) {
	svc := faceService(t, &AccountQuota{
		FetchedAt: time.Now(),
		Secondary: &QuotaWindow{Label: "Weekly", UsedPercent: 10},
	})
	suid := "/cliproxy/account/k1"
	f, ok := svc.QuotaFace(suid, ReadingIDFor(suid, "/quota/session/used/0"))
	if !ok {
		t.Fatal("expected a face")
	}
	if len(f.Windows) != 1 || f.Windows[0].Label != "Weekly" {
		t.Fatalf("windows = %+v", f.Windows)
	}
}

func TestQuotaFaceLongAccountLabel(t *testing.T) {
	svc := NewServiceWithClient(nil)
	svc.accounts["k2"] = Account{
		AuthIndex: flexString("k2"),
		Provider:  "codex",
		Email:     "pauloudex@example.com",
		Status:    "ready",
	}
	svc.quotas["k2"] = &AccountQuota{
		FetchedAt: time.Now(),
		Primary:   &QuotaWindow{Label: "Session", UsedPercent: 10},
	}
	suid := "/cliproxy/account/k2"
	f, ok := svc.QuotaFace(suid, ReadingIDFor(suid, "/quota/session/used/0"))
	if !ok {
		t.Fatal("expected a face")
	}
	// The header is the provider alone; the account survives as LabelShort
	// for the header truncation fallback.
	if f.Label != "CODEX" || f.LabelShort != "PAULOUDEX" {
		t.Fatalf("labels = %q / %q", f.Label, f.LabelShort)
	}
}

func TestQuotaFaceRejectsNonQuotaReadings(t *testing.T) {
	svc := faceService(t, &AccountQuota{
		FetchedAt: time.Now(),
		Primary:   &QuotaWindow{Label: "Session", UsedPercent: 10},
	})
	suid := "/cliproxy/account/k1"
	for _, path := range []string{"/status/0", "/quota/session/reset/0", "/requests/0"} {
		if _, ok := svc.QuotaFace(suid, ReadingIDFor(suid, path)); ok {
			t.Fatalf("reading %s should not resolve to a face", path)
		}
	}
	if _, ok := svc.QuotaFace("/cliproxy/proxy", 1); ok {
		t.Fatal("proxy summary sensor should not resolve")
	}
	if _, ok := svc.QuotaFace("/cliproxy/account/unknown", ReadingIDFor("/cliproxy/account/unknown", "/quota/session/used/0")); ok {
		t.Fatal("unknown account should not resolve")
	}
}

func TestQuotaFaceNoQuotaYet(t *testing.T) {
	svc := faceService(t, nil)
	suid := "/cliproxy/account/k1"
	f, ok := svc.QuotaFace(suid, ReadingIDFor(suid, "/quota/session/used/0"))
	if !ok {
		t.Fatal("a bound quota reading should still get a face")
	}
	if len(f.Windows) != 0 || !f.Stale {
		t.Fatalf("empty face = windows %d stale %v", len(f.Windows), f.Stale)
	}
}

func TestQuotaFaceStaleFlag(t *testing.T) {
	q := &AccountQuota{
		FetchedAt: time.Now().Add(-3 * quotaInterval),
		Primary:   &QuotaWindow{Label: "Session", UsedPercent: 10},
	}
	svc := faceService(t, q)
	suid := "/cliproxy/account/k1"
	f, _ := svc.QuotaFace(suid, ReadingIDFor(suid, "/quota/session/used/0"))
	if !f.Stale {
		t.Fatal("old snapshot should be stale")
	}
	if len(f.Windows) != 1 {
		t.Fatalf("stale face should still carry windows, got %d", len(f.Windows))
	}
}

func TestCompactReset(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		at   *time.Time
		want string
	}{
		{"nil", nil, ""},
		{"overdue", ptrTime(now.Add(-time.Minute)), "NOW"},
		{"minutes", ptrTime(now.Add(45 * time.Minute)), "45M"},
		{"hours", ptrTime(now.Add(2*time.Hour + 14*time.Minute)), "2H14M"},
		{"days", ptrTime(now.Add(55*time.Hour + 7*time.Hour)), "2D14H"},
		{"under a minute", ptrTime(now.Add(20 * time.Second)), "1M"},
	}
	for _, c := range cases {
		if got := CompactReset(c.at, now); got != c.want {
			t.Fatalf("%s: CompactReset = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWindowTag(t *testing.T) {
	cases := map[string]string{
		"Session": "5H", "session": "5H",
		"Daily": "DY", "Weekly": "WK", "Monthly": "MO",
		"Rolling": "?", "": "?",
	}
	for label, want := range cases {
		if got := WindowTag(label); got != want {
			t.Fatalf("WindowTag(%q) = %q, want %q", label, got, want)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
