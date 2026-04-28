package handler

import (
	"context"
	"database/sql"
	"fmt"
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

// ─── Setup ──────────────────────────────────────────────────────────────────

func setupTestServer(t *testing.T) (*httptest.Server, *sessions.CookieStore, *store.SQLStore) {
	t.Helper()

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

// newClient crea un http.Client con cookie jar que sigue redirects
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// loginUser registra y loguea un usuario, devuelve el client autenticado
func loginUser(t *testing.T, serverURL, email, password string, appStore *store.SQLStore) *http.Client {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.CreateUser(context.Background(), email, string(hash)); err != nil {
		t.Fatal(err)
	}
	client := newClient(t)
	resp, err := client.PostForm(serverURL+"/login", url.Values{
		"email":    {email},
		"password": {password},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login falló: status %d en %s", resp.StatusCode, resp.Request.URL)
	}
	return client
}

// createSpace crea un espacio vía POST y devuelve su ID consultando el store
func createSpace(t *testing.T, client *http.Client, serverURL string, appStore *store.SQLStore, userEmail string) string {
	t.Helper()
	resp, err := client.PostForm(serverURL+"/espacios", url.Values{
		"nombre":               {"Sala Test"},
		"tipo":                 {"Reunión"},
		"hora_apertura":        {"08:00"},
		"hora_cierre":          {"18:00"},
		"duracion_min_minutos": {"60"},
		"precio_hora":          {"100"},
		"recargo_fin_semana":   {"0.20"},
		"descuento_volumen":    {"0.10"},
		"horas_para_descuento": {"4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Obtener el usuario para buscar su espacio
	user, err := appStore.GetUserByEmail(context.Background(), userEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	spaces, err := appStore.ListSpacesByUser(context.Background(), user.ID)
	if err != nil || len(spaces) == 0 {
		t.Fatalf("no se encontró espacio para usuario %s: %v", userEmail, err)
	}
	return fmt.Sprintf("%d", spaces[0].ID)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestLoginAndCreateSpace(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "user@example.com", "secret123", appStore)

	spaceID := createSpace(t, client, server.URL, appStore, "user@example.com")
	if spaceID == "" {
		t.Fatal("no se obtuvo ID del espacio creado")
	}

	resp, err := client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "Sala Test") {
		t.Fatalf("esperaba 'Sala Test' en la lista, body:\n%s", string(body)[:min(len(string(body)), 500)])
	}
}

func TestRegisterAndLogin(t *testing.T) {
	server, _, _ := setupTestServer(t)
	defer server.Close()

	client := newClient(t)
	resp, err := client.PostForm(server.URL+"/registro", url.Values{
		"email":    {"nuevo@example.com"},
		"password": {"mipassword"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200 tras registro, obtuve %d", resp.StatusCode)
	}

	resp, err = client.PostForm(server.URL+"/login", url.Values{
		"email":    {"nuevo@example.com"},
		"password": {"mipassword"},
	})
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

	client := newClient(t)
	resp, err := client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(resp.Request.URL.Path, "login") {
		t.Fatalf("esperaba redirect a /login, URL final: %s", resp.Request.URL)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	hash, _ := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.DefaultCost)
	appStore.CreateUser(context.Background(), "u@example.com", string(hash))

	client := newClient(t)
	// Contraseña incorrecta
	resp, err := client.PostForm(server.URL+"/login", url.Values{
		"email":    {"u@example.com"},
		"password": {"wrongpass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "nv") { // "inválidas"
		t.Error("esperaba mensaje de error de credenciales")
	}
}

func TestGetForms(t *testing.T) {
	server, _, _ := setupTestServer(t)
	defer server.Close()

	client := newClient(t)

	for _, path := range []string{"/login", "/registro"} {
		resp, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: esperaba 200, obtuve %d", path, resp.StatusCode)
		}
		if !strings.Contains(string(body), "ReservaEspacios") {
			t.Errorf("GET %s: esperaba contenido HTML", path)
		}
	}
}

func TestNewSpaceForm(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u@example.com", "pass123", appStore)

	resp, err := client.Get(server.URL + "/espacios/nuevo")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "espacio") {
		t.Error("esperaba formulario de nuevo espacio")
	}
}

func TestSpaceDetail(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u2@example.com", "pass123", appStore)
	spaceID := createSpace(t, client, server.URL, appStore, "u2@example.com")

	resp, err := client.Get(server.URL + "/espacios/" + spaceID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Sala Test") {
		t.Error("esperaba nombre del espacio en el detalle")
	}
}

func TestEditSpaceFormAndUpdate(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u3@example.com", "pass123", appStore)
	spaceID := createSpace(t, client, server.URL, appStore, "u3@example.com")

	// GET formulario de edición
	resp, err := client.Get(server.URL + "/espacios/" + spaceID + "/editar")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET editar: esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Sala Test") {
		t.Error("esperaba nombre del espacio en el formulario de edición")
	}

	// POST actualización
	resp, err = client.PostForm(server.URL+"/espacios/"+spaceID+"/editar", url.Values{
		"nombre":               {"Sala Actualizada"},
		"tipo":                 {"Oficina"},
		"hora_apertura":        {"09:00"},
		"hora_cierre":          {"19:00"},
		"duracion_min_minutos": {"60"},
		"precio_hora":          {"120"},
		"recargo_fin_semana":   {"0.15"},
		"descuento_volumen":    {"0.05"},
		"horas_para_descuento": {"3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Sala Actualizada") {
		t.Errorf("esperaba 'Sala Actualizada' en respuesta, body: %s", string(body)[:min(len(string(body)), 300)])
	}
}

func TestCreateReserva(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u4@example.com", "pass123", appStore)
	spaceID := createSpace(t, client, server.URL, appStore, "u4@example.com")

	resp, err := client.PostForm(server.URL+"/espacios/"+spaceID+"/reservas", url.Values{
		"fecha":       {"2026-06-15"},
		"hora_inicio": {"10:00"},
		"hora_fin":    {"12:00"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// Debe mostrar el detalle con la reserva o mensaje de éxito
	if resp.StatusCode != http.StatusOK {
		t.Errorf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	_ = body
}

func TestAvailabilityAPI(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u5@example.com", "pass123", appStore)
	spaceID := createSpace(t, client, server.URL, appStore, "u5@example.com")

	apiURL := fmt.Sprintf("%s/api/reservas/disponibilidad?espacio_id=%s&fecha=2026-07-01&hora_inicio=10:00&hora_fin=12:00", server.URL, spaceID)
	resp, err := client.Get(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("availability API: esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "available") {
		t.Errorf("esperaba campo 'available' en JSON, body: %s", string(body))
	}
}

func TestPriceAPI(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u6@example.com", "pass123", appStore)
	spaceID := createSpace(t, client, server.URL, appStore, "u6@example.com")

	apiURL := fmt.Sprintf("%s/api/reservas/precio?espacio_id=%s&fecha=2026-07-01&hora_inicio=10:00&hora_fin=12:00", server.URL, spaceID)
	resp, err := client.Get(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("price API: esperaba 200, obtuve %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "price") {
		t.Errorf("esperaba campo 'price' en JSON, body: %s", string(body))
	}
}

func TestLogout(t *testing.T) {
	server, _, appStore := setupTestServer(t)
	defer server.Close()

	client := loginUser(t, server.URL, "u7@example.com", "pass123", appStore)

	// Logout
	resp, err := client.PostForm(server.URL+"/logout", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Tras logout, /espacios debe redirigir a /login
	resp, err = client.Get(server.URL + "/espacios")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(resp.Request.URL.Path, "login") {
		t.Errorf("tras logout, esperaba redirect a /login, URL: %s", resp.Request.URL)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
