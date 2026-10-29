package utils

import "testing"

func TestUniqueName(t *testing.T) {
	t.Run("first_occurrence_returned_as_is", func(t *testing.T) {
		counts := map[string]int{}
		got := UniqueName("photo.jpg", counts)
		if got != "photo.jpg" {
			t.Errorf("first call = %q, want %q", got, "photo.jpg")
		}
	})

	t.Run("second_occurrence_appends_2", func(t *testing.T) {
		counts := map[string]int{}
		_ = UniqueName("photo.jpg", counts)
		got := UniqueName("photo.jpg", counts)
		if got != "photo_2.jpg" {
			t.Errorf("second call = %q, want %q", got, "photo_2.jpg")
		}
	})

	t.Run("third_occurrence_appends_3", func(t *testing.T) {
		counts := map[string]int{}
		_ = UniqueName("doc.pdf", counts)
		_ = UniqueName("doc.pdf", counts)
		got := UniqueName("doc.pdf", counts)
		if got != "doc_3.pdf" {
			t.Errorf("third call = %q, want %q", got, "doc_3.pdf")
		}
	})

	t.Run("no_extension_appends_counter_to_base", func(t *testing.T) {
		counts := map[string]int{}
		_ = UniqueName("README", counts)
		got := UniqueName("README", counts)
		if got != "README_2" {
			t.Errorf("got %q, want %q", got, "README_2")
		}
	})

	t.Run("counts_per_name_are_independent", func(t *testing.T) {
		counts := map[string]int{}
		a1 := UniqueName("a.txt", counts)
		b1 := UniqueName("b.txt", counts)
		a2 := UniqueName("a.txt", counts)
		if a1 != "a.txt" || b1 != "b.txt" || a2 != "a_2.txt" {
			t.Errorf("got (%q, %q, %q); want (a.txt, b.txt, a_2.txt)", a1, b1, a2)
		}
	})

	t.Run("multi_dot_extension_uses_last_segment", func(t *testing.T) {
		counts := map[string]int{}
		_ = UniqueName("archive.tar.gz", counts)
		got := UniqueName("archive.tar.gz", counts)
		if got != "archive.tar_2.gz" {
			t.Errorf("got %q, want %q", got, "archive.tar_2.gz")
		}
	})
}
