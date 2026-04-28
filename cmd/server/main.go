package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"proyecto-monolito/internal/auth"
	"proyecto-monolito/internal/handler"
	"proyecto-monolito/internal/store"
	"proyecto-monolito/internal/template"
)

func main() {
	port := getenv("PORT", "8080")
	dbPath := getenv("DB_PATH", "data.db")
	secret := getenv("APP_SECRET", "clave-super-secreta")
	https := getenv("HTTPS", "false") == "true"

	if err := runServer(port, dbPath, secret, https); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func runServer(port, dbPath, secret string, secureCookies bool) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil && filepath.Dir(dbPath) != "." {
		return err
	}

	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	goose.SetDialect("sqlite3")
	if err := goose.Up(dbConn, "migrations"); err != nil {
		return err
	}

	store := store.NewStore(dbConn)
	renderer, err := template.NewRenderer()
	if err != nil {
		return err
	}

	sessionStore := sessions.NewCookieStore([]byte(secret))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 días
		HttpOnly: true,
		Secure:   secureCookies, // true en producción con HTTPS
		SameSite: http.SameSiteLaxMode,
	}
	authService := auth.NewAuth(store, sessionStore)
	h := handler.NewHandler(store, authService, renderer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		h.RedirectToSpaces(w, r)
	})

	r.Get("/registro", h.RegisterForm)
	r.Post("/registro", h.RegisterSubmit)
	r.Get("/login", h.LoginForm)
	r.Post("/login", h.LoginSubmit)
	r.Post("/logout", h.Logout)

	r.Route("/espacios", func(r chi.Router) {
		r.Use(authService.RequireAuth)
		r.Get("/", h.ListSpaces)
		r.Get("/nuevo", h.NewSpaceForm)
		r.Post("/", h.CreateSpace)
		r.Get("/{id}", h.SpaceDetail)
		r.Get("/{id}/editar", h.EditSpaceForm)
		r.Post("/{id}/editar", h.UpdateSpace)
		r.Post("/{id}/reservas", h.CreateReserva)
		r.Post("/{id}/reservas/{reservaID}/cancelar", h.CancelReserva)
	})

	r.With(authService.RequireAuth).Get("/api/reservas/disponibilidad", h.AvailabilityJSON)
	r.With(authService.RequireAuth).Get("/api/reservas/precio", h.PriceJSON)

	return http.ListenAndServe(":"+port, r)
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
