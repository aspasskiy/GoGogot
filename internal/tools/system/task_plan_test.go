package system

import (
	"strings"
	"testing"
)

func TestParseTaskStatus(t *testing.T) {
	cases := []struct {
		in    string
		want  taskStatus
		valid bool
	}{
		{"pending", taskPending, true},
		{"in_progress", taskInProgress, true},
		{"completed", taskCompleted, true},
		{"", "", false},
		{"PENDING", "", false},
		{"done", "", false},
	}
	for _, c := range cases {
		got, valid := parseTaskStatus(c.in)
		if valid != c.valid || got != c.want {
			t.Errorf("parseTaskStatus(%q) = (%q,%v); want (%q,%v)",
				c.in, got, valid, c.want, c.valid)
		}
	}
}

func TestTaskPlan_Create(t *testing.T) {
	t.Run("creates_with_default_pending_status", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.create([]map[string]any{
			{"title": "first"},
			{"title": "second"},
		})
		if r.IsErr {
			t.Fatalf("create errored: %s", r.Output)
		}
		if !strings.Contains(r.Output, "Created 2") {
			t.Errorf("unexpected output: %q", r.Output)
		}
		snap := tp.Snapshot()
		if len(snap) != 2 {
			t.Fatalf("snapshot len = %d, want 2", len(snap))
		}
		if snap[0].Title != "first" || string(snap[0].Status) != "pending" {
			t.Errorf("first task = %+v", snap[0])
		}
	})

	t.Run("create_replaces_existing_list", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.create([]map[string]any{{"title": "old"}})
		tp.create([]map[string]any{{"title": "new"}})
		snap := tp.Snapshot()
		if len(snap) != 1 || snap[0].Title != "new" {
			t.Errorf("snapshot = %+v, want only \"new\"", snap)
		}
	})

	t.Run("create_with_explicit_status", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.create([]map[string]any{
			{"title": "a", "status": "in_progress"},
		})
		if r.IsErr {
			t.Fatalf("create errored: %s", r.Output)
		}
		snap := tp.Snapshot()
		if string(snap[0].Status) != "in_progress" {
			t.Errorf("status = %q, want in_progress", snap[0].Status)
		}
	})

	t.Run("create_rejects_empty_title", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.create([]map[string]any{{"title": ""}})
		if !r.IsErr {
			t.Errorf("expected error for empty title")
		}
	})

	t.Run("create_rejects_invalid_status", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.create([]map[string]any{{"title": "a", "status": "done"}})
		if !r.IsErr {
			t.Errorf("expected error for invalid status %q", "done")
		}
	})

	t.Run("create_resets_nextID_to_match_count_plus_one", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.create([]map[string]any{{"title": "a"}, {"title": "b"}, {"title": "c"}})
		r := tp.add("d")
		if !strings.Contains(r.Output, "[4]") {
			t.Errorf("after create-3 + add, expected ID=4; got %q", r.Output)
		}
	})
}

func TestTaskPlan_Add(t *testing.T) {
	t.Run("appends_with_incrementing_ids", func(t *testing.T) {
		tp := NewTaskPlan()
		r1 := tp.add("first")
		r2 := tp.add("second")
		if !strings.Contains(r1.Output, "[1]") {
			t.Errorf("first add output = %q, want id 1", r1.Output)
		}
		if !strings.Contains(r2.Output, "[2]") {
			t.Errorf("second add output = %q, want id 2", r2.Output)
		}
	})

	t.Run("rejects_empty_title", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.add("")
		if !r.IsErr {
			t.Errorf("expected error for empty title")
		}
	})

	t.Run("status_defaults_to_pending", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.add("x")
		snap := tp.Snapshot()
		if string(snap[0].Status) != "pending" {
			t.Errorf("status = %q, want pending", snap[0].Status)
		}
	})
}

func TestTaskPlan_Update(t *testing.T) {
	t.Run("updates_existing_task_status", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.add("x")
		r := tp.update(1, "completed")
		if r.IsErr {
			t.Fatalf("update errored: %s", r.Output)
		}
		snap := tp.Snapshot()
		if string(snap[0].Status) != "completed" {
			t.Errorf("status = %q, want completed", snap[0].Status)
		}
	})

	t.Run("rejects_invalid_status", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.add("x")
		r := tp.update(1, "bogus")
		if !r.IsErr {
			t.Errorf("expected error for invalid status")
		}
	})

	t.Run("rejects_unknown_id", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.add("x")
		r := tp.update(999, "completed")
		if !r.IsErr {
			t.Errorf("expected error for unknown id")
		}
	})
}

func TestTaskPlan_List(t *testing.T) {
	t.Run("empty_returns_no_tasks", func(t *testing.T) {
		tp := NewTaskPlan()
		r := tp.list()
		if r.Output != "No tasks." {
			t.Errorf("got %q, want %q", r.Output, "No tasks.")
		}
	})

	t.Run("populated_shows_completion_count_and_each_task", func(t *testing.T) {
		tp := NewTaskPlan()
		tp.add("a")
		tp.add("b")
		tp.update(1, "completed")
		out := tp.list().Output
		if !strings.Contains(out, "(1/2 completed)") {
			t.Errorf("missing 1/2 count in %q", out)
		}
		if !strings.Contains(out, "[1] [completed] a") {
			t.Errorf("missing first task line in %q", out)
		}
		if !strings.Contains(out, "[2] [pending] b") {
			t.Errorf("missing second task line in %q", out)
		}
	})
}

func TestTaskPlan_DeleteAll(t *testing.T) {
	tp := NewTaskPlan()
	tp.add("a")
	tp.add("b")
	tp.deleteAll()
	if got := len(tp.Snapshot()); got != 0 {
		t.Errorf("after deleteAll snapshot len = %d, want 0", got)
	}
	r := tp.add("fresh")
	if !strings.Contains(r.Output, "[1]") {
		t.Errorf("after deleteAll id should reset to 1; got %q", r.Output)
	}
}

func TestTaskPlan_Snapshot_EmptyReturnsNil(t *testing.T) {
	tp := NewTaskPlan()
	if snap := tp.Snapshot(); snap != nil {
		t.Errorf("empty Snapshot = %+v, want nil", snap)
	}
}
