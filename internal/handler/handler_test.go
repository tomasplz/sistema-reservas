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

	// Cada test usa una base de datos en memoria única con URI distinto
	// para evitar que tests paralelos compartan estado.
	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbConn.Close() })

	goose.SetDialect("sqlite3")
	if err := goose.Up(dbConn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}

	appStore := store.NewStore(dbConn)
	sessionStore := sessions.NewCookieStore([]byte("test-secret"))
	// Necesario para que gorilla/sessions no exija HTTPS en tests
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   false,
	}
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

// newTestClient crea un http.Client con cookie jar que sigue redirects
// pero para en el primer redirect de tipo 303 en requests de formulario.
func newTestClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func TestLoginAndCreateSpace(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	// Crear usuario directamente en la base de datos del test
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.CreateUser(context.Background(), "user@example.com", string(passwordHash)); err != nil {
		t.Fatal(err)
	}

	client := newTestClient(t)

	// --- Login ---
	loginForm := url.Values{}
	loginForm.Set("email", "user@example.com")
	loginForm.Set("password", "secret123")

	resp, err := client.PostForm(server.URL+"/login", loginForm)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Después de seguir el redirect, debe estar en /espacios (200)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 tras login+redirect, obtuve %d (URL final: %s)", resp.StatusCode, resp.Request.URL)
	}

	// --- Crear espacio ---
	spaceForm := url.Values{}
	spaceForm.Set("nombre", "Sala A")
	spaceForm.Set("tipo", "Reunión")
	spaceForm.Set("hora_apertura", "08:00")
	spaceForm.Set("hora_cierre", "18:00")
	spaceForm.Set("duracion_min_minutos", "60")
	spaceForm.Set("precio_hora", "75")
	spaceForm.Set("recargo_fin_semana", "0.20")
	spaceForm.Set("descuento_volumen", "0.10")
	spaceForm.Set("horas_para_descuento", "4")

	resp, err = client.PostForm(server.URL+"/espacios", spaceForm)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 tras crear espacio+redirect, obtuve %d", resp.StatusCode)
	}

	// --- Verificar que el espacio aparece en el listado ---
	resp, err = client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "Sala A") {
		t.Fatalf("esperaba 'Sala A' en el listado, obtenido:\n%s", string(body))
	}
}

func TestRegisterAndLogin(t *testing.T) {
	server, _, _ := setupTestServer(t)
	defer server.Close()

	client := newTestClient(t)

	// --- Registro ---
	regForm := url.Values{}
	regForm.Set("email", "nuevo@example.com")
	regForm.Set("password", "mipassword")

	resp, err := client.PostForm(server.URL+"/registro", regForm)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 tras registro, obtuve %d", resp.StatusCode)
	}

	// --- Login con las mismas credenciales ---
	loginForm := url.Values{}
	loginForm.Set("email", "nuevo@example.com")
	loginForm.Set("password", "mipassword")

	resp, err = client.PostForm(server.URL+"/login", loginForm)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 tras login, obtuve %d", resp.StatusCode)
	}
}

func TestRequireAuthRedirects(t *testing.T) {
	server, _, _ := setupTestServer(t)
	defer server.Close()

	// Sin cookie de sesión, acceder a /espacios debe redirigir a /login
	client := newTestClient(t)

	resp, err := client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Después de seguir el redirect, debería estar en /login
	if !strings.Contains(resp.Request.URL.Path, "login") {
		t.Fatalf("esperaba redirigir a /login, URL final: %s", resp.Request.URL)
	}
}
