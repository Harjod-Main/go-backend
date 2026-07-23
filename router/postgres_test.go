package router

import (
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/config"
)

func TestPostgresDSN_FromDiscreteFields(t *testing.T) {
	dsn, err := postgresDSN(config.Postgres{
		Host:     "db.example.supabase.co",
		Port:     "5432",
		Name:     "postgres",
		User:     "postgres",
		Password: "secret",
		SSLMode:  "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "db.example.supabase.co:5432") {
		t.Fatalf("unexpected host in dsn: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("expected sslmode=require in dsn: %s", dsn)
	}
}

func TestPostgresDSN_PrefersDatabaseURL(t *testing.T) {
	want := "postgres://u:p@host:5432/db?sslmode=require"
	dsn, err := postgresDSN(config.Postgres{
		DatabaseURL: want,
		Host:        "ignored",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != want {
		t.Fatalf("got %q want %q", dsn, want)
	}
}
