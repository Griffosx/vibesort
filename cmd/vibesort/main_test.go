package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibesort/internal/inventory"
	"vibesort/internal/plan"
)

func TestRunInventory(t *testing.T) {
	sourcePath := writeTempFile(t, "x.go", "package p\n\nfunc A() {}\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"inventory", sourcePath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var doc inventory.Document
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.File != sourcePath || doc.ReadyPlan.SchemaVersion != inventory.PlanSchemaVersion {
		t.Fatalf("inventory output = %#v", doc)
	}
}

func TestRunValidateValidPlan(t *testing.T) {
	sourcePath := writeTempFile(t, "x.go", "package p\n\nfunc A() {}\n")
	planPath := writePlanFile(t, sourcePath, "func:A")

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", planPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var result plan.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.File != sourcePath || result.EntityCount != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunValidateInvalidPlan(t *testing.T) {
	sourcePath := writeTempFile(t, "x.go", "package p\n\nfunc A() {}\n\nfunc B() {}\n")
	planPath := writePlanFile(t, sourcePath, "func:A", "func:A")

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", planPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, `vibesort: duplicate entity id "func:A"`) {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing args",
			args: nil,
			want: usage,
		},
		{
			name: "inventory wrong arg count",
			args: []string{"inventory"},
			want: inventoryUsage,
		},
		{
			name: "validate wrong arg count",
			args: []string{"validate"},
			want: validateUsage,
		},
		{
			name: "unknown subcommand",
			args: []string{"wat"},
			want: usage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr = %q, want containing %q", stderr.String(), tc.want)
			}
		})
	}
}

func writePlanFile(t *testing.T, sourcePath string, order ...string) string {
	t.Helper()
	data, err := json.Marshal(plan.Plan{
		SchemaVersion: inventory.PlanSchemaVersion,
		File:          sourcePath,
		Order:         orderItems(order),
	})
	if err != nil {
		t.Fatal(err)
	}
	return writeTempFile(t, "plan.json", string(data))
}

func orderItems(order []string) []inventory.OrderItem {
	items := make([]inventory.OrderItem, 0, len(order))
	for _, id := range order {
		items = append(items, inventory.OrderItem{ID: id})
	}
	return items
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
