package inbox

import "testing"

func TestNormalizeWindowRecontactSettings(t *testing.T) {
	t.Parallel()

	ok, err := NormalizeWindowRecontactSettings(WindowRecontactSettings{
		Enabled:    true,
		Text:       "  Halo kak  ",
		AfterHours: 20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok.Text != "Halo kak" || ok.AfterHours != 20 || !ok.Enabled {
		t.Fatalf("unexpected normalized settings: %+v", ok)
	}

	if _, err := NormalizeWindowRecontactSettings(WindowRecontactSettings{
		Enabled:    true,
		Text:       " ",
		AfterHours: 12,
	}); err == nil {
		t.Fatal("expected validation error when enabled without text")
	}

	if _, err := NormalizeWindowRecontactSettings(WindowRecontactSettings{
		Enabled:    false,
		Text:       "x",
		AfterHours: 24,
	}); err == nil {
		t.Fatal("expected validation error for after_hours > 23")
	}

	if _, err := NormalizeWindowRecontactSettings(WindowRecontactSettings{
		Enabled:    false,
		Text:       "x",
		AfterHours: 0,
	}); err == nil {
		t.Fatal("expected validation error for after_hours < 1")
	}

	defaults, err := NormalizeWindowRecontactSettings(WindowRecontactSettings{
		Enabled:    false,
		Text:       "",
		AfterHours: 23,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaults.Text != DefaultWindowRecontactText {
		t.Fatalf("expected default text, got %q", defaults.Text)
	}
}
