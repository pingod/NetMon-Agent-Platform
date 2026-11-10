package repos

import (
	"os"
	"testing"

	"itops-backend/internal/appdb"
	"itops-backend/internal/migrate"
)

func TestServerCreate(t *testing.T) {
	tmpDb := "test_servers.db"
	defer os.Remove(tmpDb)

	if err := appdb.Open(tmpDb); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer appdb.Close()

	if err := migrate.RunMigrations(appdb.DB()); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	req := ServerCreateInput{
		Name:        "Test Server",
		Hostname:    "192.168.1.50",
		Port:        22,
		Username:    "root",
		Password:    "password123",
		OSType:      "linux",
		Vendor:      "other",
		Description: "Test Desc",
	}

	s, err := Servers.Create(req)
	if err != nil {
		t.Fatalf("Servers.Create failed: %v", err)
	}
	t.Logf("Created server successfully: %+v", s)

	list, err := Servers.List()
	if err != nil {
		t.Fatalf("Servers.List failed: %v", err)
	}
	t.Logf("Server list len: %d", len(list))

	got, err := Servers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("Servers.GetByID failed: %v", err)
	}
	t.Logf("GetByID: %+v", got)
}
