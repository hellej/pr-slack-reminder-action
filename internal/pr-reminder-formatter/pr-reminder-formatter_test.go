package prreminderformatter_test

import "testing"

func TestFoo(t *testing.T) {
	t.Run("Example case", func(t *testing.T) {
		got := 1 + 1
		want := 2
		if got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	})
}
