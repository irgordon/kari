package db

import "testing"

func TestLoadMigrationPlan(t *testing.T) {
	plan, err := LoadMigrationPlan()
	if err != nil {
		t.Fatalf("LoadMigrationPlan() error = %v", err)
	}
	if len(plan) != 4 {
		t.Fatalf("LoadMigrationPlan() count = %d, want 4", len(plan))
	}
	for index, migration := range plan {
		wantVersion := int64(index + 1)
		if migration.Version != wantVersion {
			t.Errorf("migration %s version = %d, want %d", migration.Name, migration.Version, wantVersion)
		}
		if migration.Checksum == "" {
			t.Errorf("migration %s has empty checksum", migration.Name)
		}
	}
}
