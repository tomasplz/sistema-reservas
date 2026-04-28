package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"proyecto-monolito/internal/store"
)

func migrationsPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

func setupDB(t *testing.T) *store.SQLStore {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	goose.SetDialect("sqlite3")
	if err := goose.Up(db, migrationsPath()); err != nil {
		t.Fatal(err)
	}
	return store.NewStore(db)
}

func TestCreateAndGetUser(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "test@example.com", "hash123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("email: got %q, want %q", user.Email, "test@example.com")
	}

	got, err := s.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("IDs no coinciden: got %d, want %d", got.ID, user.ID)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	s := setupDB(t)
	_, err := s.GetUserByEmail(context.Background(), "noexiste@example.com")
	if err == nil {
		t.Fatal("esperaba error para usuario no existente")
	}
}

func TestCreateAndListSpaces(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "owner@example.com", "hash")

	space, err := s.CreateSpace(ctx, user.ID, "Sala Test", "Sala",
		"08:00", "18:00", 60, 100.0, 0.20, 0.10, 4)
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if space.Nombre != "Sala Test" {
		t.Errorf("nombre: got %q, want %q", space.Nombre, "Sala Test")
	}

	spaces, err := s.ListSpacesByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListSpacesByUser: %v", err)
	}
	if len(spaces) != 1 {
		t.Errorf("esperaba 1 espacio, obtuve %d", len(spaces))
	}
}

func TestGetSpaceByID(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "u@example.com", "hash")
	created, _ := s.CreateSpace(ctx, user.ID, "Sala X", "Oficina",
		"09:00", "17:00", 30, 50.0, 0.0, 0.0, 0)

	got, err := s.GetSpaceByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSpaceByID: %v", err)
	}
	if got.Nombre != "Sala X" {
		t.Errorf("nombre: got %q, want %q", got.Nombre, "Sala X")
	}
}

func TestGetSpaceByID_NotFound(t *testing.T) {
	s := setupDB(t)
	_, err := s.GetSpaceByID(context.Background(), 99999)
	if err == nil {
		t.Fatal("esperaba error para ID inexistente")
	}
}

func TestUpdateSpace(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "u2@example.com", "hash")
	space, _ := s.CreateSpace(ctx, user.ID, "Original", "Sala",
		"08:00", "18:00", 60, 50.0, 0.0, 0.0, 0)

	err := s.UpdateSpace(ctx, "Actualizada", "Oficina",
		"09:00", "19:00", 90, 75.0, 0.10, 0.05, 2, space.ID)
	if err != nil {
		t.Fatalf("UpdateSpace: %v", err)
	}

	updated, _ := s.GetSpaceByID(ctx, space.ID)
	if updated.Nombre != "Actualizada" {
		t.Errorf("nombre actualizado: got %q, want %q", updated.Nombre, "Actualizada")
	}
}

func TestCreateAndListReservas(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "r@example.com", "hash")
	space, _ := s.CreateSpace(ctx, user.ID, "Sala R", "Sala",
		"08:00", "18:00", 60, 100.0, 0.0, 0.0, 0)

	reserva, err := s.CreateReserva(ctx, space.ID, "2026-05-01", "10:00", "12:00", 200.0)
	if err != nil {
		t.Fatalf("CreateReserva: %v", err)
	}
	if reserva.PrecioTotal != 200.0 {
		t.Errorf("precio: got %.2f, want %.2f", reserva.PrecioTotal, 200.0)
	}

	reservas, err := s.ListReservasBySpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("ListReservasBySpace: %v", err)
	}
	if len(reservas) != 1 {
		t.Errorf("esperaba 1 reserva, obtuve %d", len(reservas))
	}
}

func TestGetReservasBySpaceAndDate(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "d@example.com", "hash")
	space, _ := s.CreateSpace(ctx, user.ID, "Sala D", "Sala",
		"08:00", "18:00", 60, 100.0, 0.0, 0.0, 0)

	s.CreateReserva(ctx, space.ID, "2026-05-10", "10:00", "11:00", 100.0)
	s.CreateReserva(ctx, space.ID, "2026-05-11", "10:00", "11:00", 100.0)

	reservas, err := s.GetReservasBySpaceAndDate(ctx, space.ID, "2026-05-10")
	if err != nil {
		t.Fatalf("GetReservasBySpaceAndDate: %v", err)
	}
	if len(reservas) != 1 {
		t.Errorf("esperaba 1 reserva en esa fecha, obtuve %d", len(reservas))
	}
}

func TestCancelReserva(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "c@example.com", "hash")
	space, _ := s.CreateSpace(ctx, user.ID, "Sala C", "Sala",
		"08:00", "18:00", 60, 100.0, 0.0, 0.0, 0)
	reserva, _ := s.CreateReserva(ctx, space.ID, "2026-06-01", "10:00", "11:00", 100.0)

	if err := s.CancelReserva(ctx, reserva.ID); err != nil {
		t.Fatalf("CancelReserva: %v", err)
	}

	reservas, _ := s.ListReservasBySpace(ctx, space.ID)
	if len(reservas) != 0 {
		t.Errorf("esperaba 0 reservas tras cancelar, obtuve %d", len(reservas))
	}
}

func TestGetReservaByID(t *testing.T) {
	s := setupDB(t)
	ctx := context.Background()

	user, _ := s.CreateUser(ctx, "g@example.com", "hash")
	space, _ := s.CreateSpace(ctx, user.ID, "Sala G", "Sala",
		"08:00", "18:00", 60, 100.0, 0.0, 0.0, 0)
	reserva, _ := s.CreateReserva(ctx, space.ID, "2026-07-01", "14:00", "16:00", 200.0)

	got, err := s.GetReservaByID(ctx, reserva.ID)
	if err != nil {
		t.Fatalf("GetReservaByID: %v", err)
	}
	if got.ID != reserva.ID {
		t.Errorf("ID: got %d, want %d", got.ID, reserva.ID)
	}
}
