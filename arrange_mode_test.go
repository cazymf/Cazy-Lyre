package main

import "testing"

func TestLyreArrangeMode(t *testing.T) {
	cases := map[string]string{"": lyreArrangeStandard, "bad": lyreArrangeStandard, "standard": lyreArrangeStandard, "creative": lyreArrangeCreative}
	for input, want := range cases {
		if got := normalizeLyreArrangeMode(input); got != want {
			t.Errorf("%s: got %s want %s", input, got, want)
		}
	}
	if lyreArrangeDisplay(lyreArrangeCreative) != "Creative AI" {
		t.Fatal("creative label missing")
	}
}
