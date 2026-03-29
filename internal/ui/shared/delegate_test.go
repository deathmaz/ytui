package shared

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"max 3", "hello", 3, "hello"},
		{"max 0", "hello", 0, "hello"},
		{"negative max", "hello", -1, "hello"},
		{"empty string", "", 10, ""},
		{"unicode", "こんにちは世界", 5, "こん..."},
		{"emoji", "👍👍👍👍👍", 4, "👍..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}
