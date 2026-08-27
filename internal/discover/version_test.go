package discover

import "testing"

func TestVersionAtLeast(t *testing.T) {
	min := [3]int{7, 49, 3}
	cases := map[string]bool{
		"7.49.3":     true,
		"7.49.4":     true,
		"7.50.0":     true,
		"8.0.0":      true,
		"7.104.5":    true,
		"7.49.2":     false,
		"7.31.13":    false,
		"6.23.21":    false,
		"7.49":       false, // missing patch component defaults to 0 < 3
		"7.49.3-hf1": true,  // non-numeric suffix on the last component is tolerated
		"":           false,
		"not-a-ver":  false,
	}
	for version, want := range cases {
		if got := versionAtLeast(version, min); got != want {
			t.Errorf("versionAtLeast(%q, %v) = %v, want %v", version, min, got, want)
		}
	}
}
