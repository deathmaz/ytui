package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindCookieFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("LiteralPath", func(t *testing.T) {
		path := filepath.Join(home, "browser", "Default", "Cookies")
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("fake"), 0644)

		got, err := findCookieFile([]string{"browser/Default/Cookies"})
		if err != nil {
			t.Fatal(err)
		}
		if got != path {
			t.Errorf("got %q, want %q", got, path)
		}
	})

	t.Run("GlobPattern", func(t *testing.T) {
		profileDir := filepath.Join(home, ".mozilla", "firefox", "abc123.default-release")
		os.MkdirAll(profileDir, 0755)
		cookieFile := filepath.Join(profileDir, "cookies.sqlite")
		os.WriteFile(cookieFile, []byte("fake"), 0644)

		got, err := findCookieFile([]string{".mozilla/firefox/*.default-release/cookies.sqlite"})
		if err != nil {
			t.Fatal(err)
		}
		if got != cookieFile {
			t.Errorf("got %q, want %q", got, cookieFile)
		}
	})

	t.Run("FirstMatchWins", func(t *testing.T) {
		first := filepath.Join(home, "first", "Cookies")
		second := filepath.Join(home, "second", "Cookies")
		os.MkdirAll(filepath.Dir(first), 0755)
		os.MkdirAll(filepath.Dir(second), 0755)
		os.WriteFile(first, []byte("fake"), 0644)
		os.WriteFile(second, []byte("fake"), 0644)

		got, err := findCookieFile([]string{"first/Cookies", "second/Cookies"})
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Errorf("got %q, want first path %q", got, first)
		}
	})

	t.Run("FallsThrough", func(t *testing.T) {
		existing := filepath.Join(home, "existing", "Cookies")
		os.MkdirAll(filepath.Dir(existing), 0755)
		os.WriteFile(existing, []byte("fake"), 0644)

		got, err := findCookieFile([]string{"nonexistent/Cookies", "existing/Cookies"})
		if err != nil {
			t.Fatal(err)
		}
		if got != existing {
			t.Errorf("got %q, want %q", got, existing)
		}
	})

	t.Run("NoneFound", func(t *testing.T) {
		_, err := findCookieFile([]string{"nope1", "nope2"})
		if err == nil {
			t.Error("expected error when no paths found")
		}
	})
}

func TestBrowserConfigs(t *testing.T) {
	for _, name := range SupportedBrowsers {
		t.Run(name, func(t *testing.T) {
			cfg, ok := browserConfigs[name]
			if !ok {
				t.Fatalf("no config for browser %q", name)
			}
			if cfg.reader == nil {
				t.Error("reader is nil")
			}
			if len(cfg.paths) == 0 {
				t.Error("no cookie paths defined")
			}
		})
	}
}

func TestExtractCookies_UnsupportedBrowser(t *testing.T) {
	_, err := ExtractCookies(t.Context(), "netscape")
	if err == nil {
		t.Error("expected error for unsupported browser")
	}
}
