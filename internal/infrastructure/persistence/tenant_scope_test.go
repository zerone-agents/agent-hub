package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type scopeRow struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	Name     string
	TenantID string `gorm:"type:varchar(64);not null;default:''"`
}

func (scopeRow) TableName() string { return "scope_rows" }

func TestTenantScopes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&scopeRow{}); err != nil {
		t.Fatal(err)
	}
	rows := []scopeRow{{Name: "a", TenantID: "org-a"}, {Name: "shared", TenantID: ""}, {Name: "b", TenantID: "org-b"}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	var owned []scopeRow
	if err := TenantOwned(db, "org-a").Find(&owned).Error; err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0].Name != "a" {
		t.Fatalf("TenantOwned: got %+v, want only [a]", owned)
	}

	var withShared []scopeRow
	if err := TenantWithShared(db, "org-a").Find(&withShared).Error; err != nil {
		t.Fatal(err)
	}
	if len(withShared) != 2 {
		t.Fatalf("TenantWithShared: got %+v, want [a, shared]", withShared)
	}
	for _, r := range withShared {
		if r.Name == "b" {
			t.Fatalf("TenantWithShared leaked other tenant row: %+v", r)
		}
	}
}
