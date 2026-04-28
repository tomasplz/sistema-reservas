package auth_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"proyecto-monolito/internal/auth"
	"proyecto-monolito/internal/store"
)

func setupDB(t *testing.T) (*store.SQLStore, *sql.DB) {
	t.Helper()
	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbConn.Close() })

	_, file, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(file), "..", "..", "migrations")

	goose.SetDialect("sqlite3")
	if err := goose.Up(dbConn, migrationsPath); err != nil {
		t.Fatal(err)
	}

	return store.NewStore(dbConn), dbConn
}

func setupAuth(t *testing.T) (*auth.Auth, *store.SQLStore) {
	t.Helper()
	appStore, _ := setupDB(t)
	sessionStore := sessions.NewCookieStore([]byte("secret"))
	return auth.NewAuth(appStore, sessionStore), appStore
}

func TestAuth_RegisterAndLogin(t *testing.T) {
	a, _ := setupAuth(t)
	ctx := context.Background()

	// Registro exitoso
	userID, err := a.Register(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if userID == 0 {
		t.Error("expected valid userID, got 0")
	}

	// Login exitoso
	id, err := a.Login(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if id != userID {
		t.Errorf("Login returned wrong ID: got %d, want %d", id, userID)
	}

	// Login con clave incorrecta
	_, err = a.Login(ctx, "test@example.com", "wrongpass")
	if err == nil {
		t.Error("expected error for wrong password")
	}

	// Login con usuario inexistente
	_, err = a.Login(ctx, "noexiste@example.com", "pass")
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestAuth_SessionAndRequireAuth(t *testing.T) {
	appStore, _ := setupDB(t)
	sessionStore := sessions.NewCookieStore([]byte("secret"))
	a := auth.NewAuth(appStore, sessionStore)

	// Crear usuario mock
	userID, _ := a.Register(context.Background(), "u@example.com", "pass")

	// 1. Simular request de login (SetSessionUser)
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	if err := a.SetSessionUser(w, req, userID); err != nil {
		t.Fatalf("SetSessionUser failed: %v", err)
	}

	res := w.Result()
	cookies := res.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	// 2. Probar RequireAuth middleware con cookie válida
	req2, _ := http.NewRequest("GET", "/protected", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}

	var handlerCalled bool
	var currentUserID int64
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		currentUserID = auth.GetCurrentUserID(r)
	})

	w2 := httptest.NewRecorder()
	a.RequireAuth(nextHandler).ServeHTTP(w2, req2)

	if !handlerCalled {
		t.Error("RequireAuth didn't call next handler")
	}
	if currentUserID != userID {
		t.Errorf("expected user ID %d in context, got %d", userID, currentUserID)
	}

	// 3. Probar Logout
	req3, _ := http.NewRequest("POST", "/logout", nil)
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	w3 := httptest.NewRecorder()
	if err := a.Logout(w3, req3); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	res3 := w3.Result()
	for _, c := range res3.Cookies() {
		if c.Name == "app-session" && c.MaxAge > 0 {
			t.Error("expected session cookie to be expired")
		}
	}
}

func TestAuth_RequireAuth_Unauthorized(t *testing.T) {
	appStore, _ := setupDB(t)
	sessionStore := sessions.NewCookieStore([]byte("secret"))
	a := auth.NewAuth(appStore, sessionStore)

	// Sin cookie
	req, _ := http.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	var handlerCalled bool
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	a.RequireAuth(nextHandler).ServeHTTP(w, req)

	if handlerCalled {
		t.Error("RequireAuth should not call next handler if unauthorized")
	}
	if w.Result().StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", w.Result().StatusCode)
	}

	// Con cookie pero sin user_id
	req2, _ := http.NewRequest("GET", "/protected", nil)
	w2 := httptest.NewRecorder()
	session, _ := sessionStore.Get(req2, "app-session")
	session.Save(req2, w2)

	req3, _ := http.NewRequest("GET", "/protected", nil)
	for _, c := range w2.Result().Cookies() {
		req3.AddCookie(c)
	}
	w3 := httptest.NewRecorder()
	a.RequireAuth(nextHandler).ServeHTTP(w3, req3)

	if w3.Result().StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", w3.Result().StatusCode)
	}
}

func TestGetCurrentUserID_Empty(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	id := auth.GetCurrentUserID(req)
	if id != 0 {
		t.Errorf("expected 0, got %d", id)
	}
}

func TestAuth_Flash(t *testing.T) {
	sessionStore := sessions.NewCookieStore([]byte("secret"))
	a := auth.NewAuth(nil, sessionStore)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	if err := a.SetFlash(w, req, "test flash"); err != nil {
		t.Fatal(err)
	}

	res := w.Result()
	req2, _ := http.NewRequest("GET", "/", nil)
	for _, c := range res.Cookies() {
		req2.AddCookie(c)
	}

	w2 := httptest.NewRecorder()
	msg, err := a.GetFlash(w2, req2)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "test flash" {
		t.Fatalf("expected 'test flash', got '%s'", msg)
	}

	// Siguiente request ya no debe tener el flash
	res2 := w2.Result()
	req3, _ := http.NewRequest("GET", "/", nil)
	for _, c := range res2.Cookies() {
		req3.AddCookie(c)
	}
	w3 := httptest.NewRecorder()
	msg2, _ := a.GetFlash(w3, req3)
	if msg2 != "" {
		t.Fatalf("expected empty flash, got '%s'", msg2)
	}
}
