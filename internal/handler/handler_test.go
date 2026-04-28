package handler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"proyecto-monolito/internal/auth"
	"proyecto-monolito/internal/store"
	"proyecto-monolito/internal/template"
)

func setupTestServer(t *testing.T) (*httptest.Server, *sessions.CookieStore, *store.SQLStore) {
	t.Helper()

	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	goose.SetDialect("sqlite3")
	if err := goose.Up(dbConn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}

	appStore := store.NewStore(dbConn)
	sessionStore := sessions.NewCookieStore([]byte("test-secret"))
	authSvc := auth.NewAuth(appStore, sessionStore)
	renderer, err := template.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(appStore, authSvc, renderer)

	r := chi.NewRouter()
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.Get("/registro", h.RegisterForm)
	r.Post("/registro", h.RegisterSubmit)
	r.Get("/login", h.LoginForm)
	r.Post("/login", h.LoginSubmit)
	r.Post("/logout", h.Logout)

	r.Route("/espacios", func(r chi.Router) {
		r.Use(authSvc.RequireAuth)
		r.Get("/", h.ListSpaces)
		r.Get("/nuevo", h.NewSpaceForm)
		r.Post("/", h.CreateSpace)
		r.Get("/{id}", h.SpaceDetail)
		r.Get("/{id}/editar", h.EditSpaceForm)
		r.Post("/{id}/editar", h.UpdateSpace)
		r.Post("/{id}/reservas", h.CreateReserva)
		r.Post("/{id}/reservas/{reservaID}/cancelar", h.CancelReserva)
	})
	r.With(authSvc.RequireAuth).Get("/api/reservas/disponibilidad", h.AvailabilityJSON)
	r.With(authSvc.RequireAuth).Get("/api/reservas/precio", h.PriceJSON)

	return httptest.NewServer(r), sessionStore, appStore
}

func TestLoginAndCreateSpace(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.CreateUser(context.Background(), "user@example.com", string(passwordHash)); err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("email", "user@example.com")
	form.Set("password", "secret123")

	resp, err := client.PostForm(server.URL+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}

	form = url.Values{}
	form.Set("nombre", "Sala A")
	form.Set("tipo", "Reunión")
	form.Set("hora_apertura", "08:00")
	form.Set("hora_cierre", "18:00")
	form.Set("duracion_min_minutos", "60")
	form.Set("precio_hora", "75")
	form.Set("recargo_fin_semana", "0.20")
	form.Set("descuento_volumen", "0.10")
	form.Set("horas_para_descuento", "4")

	resp, err = client.PostForm(server.URL+"/espacios", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect after create, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Sala A") {
		t.Fatalf("expected created space in response, got body: %s", string(body))
	}
}
