package app

import "testing"

func TestOldETFReference(t *testing.T) {
	for _, value := range []string{"ETF-0042", "ETF/2018/42", "4218", ""} {
		got, err := normalizeOldETF(" " + value + " ")
		if err != nil || got != value {
			t.Fatalf("reference %q: %q %v", value, got, err)
		}
	}
	for _, value := range []string{"ETF\n42", "ETF\x0042", "ETF-你好"} {
		if _, err := normalizeOldETF(value); err == nil {
			t.Fatalf("accepted invalid barcode reference %q", value)
		}
	}
}
