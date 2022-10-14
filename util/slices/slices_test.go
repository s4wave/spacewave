package slices

import "testing"

func TestCheckSlicesContentsEqual(t *testing.T) {
	t.Run("equal string slices #1", func(t *testing.T) {
		s1 := []string{"hello2", "hello1", "hello1", "hello3", ""}
		s2 := []string{"", "hello1", "hello3", "hello2"}
		if !CheckSlicesContentsEqual(s1, s2) {
			t.Fail()
		}
	})
	t.Run("equal string slices #2", func(t *testing.T) {
		s1 := []string{"hello2"}
		s2 := []string{"hello1"}
		if CheckSlicesContentsEqual(s1, s2) {
			t.Fail()
		}
	})
	t.Run("unequal string slices #1", func(t *testing.T) {
		s1 := []string{"hello2", "hello4", "hello1", "hello3", ""}
		s2 := []string{"", "hello1", "hello3", "hello2"}
		if CheckSlicesContentsEqual(s1, s2) {
			t.Fail()
		}
	})
	t.Run("unequal string slices #2", func(t *testing.T) {
		s1 := []string{"hello2"}
		s2 := []string{"hello1"}
		if CheckSlicesContentsEqual(s1, s2) {
			t.Fail()
		}
	})
}
