package template_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	tmpl "proyecto-monolito/internal/template"
	"proyecto-monolito/internal/model"
)

func TestNewRenderer(t *testing.T) {
	r, err := tmpl.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer() error: %v", err)
	}
	if r == nil {
		t.Fatal("NewRenderer() returned nil")
	}
}

func TestRender_Login(t *testing.T) {
	r, err := tmpl.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	err = r.Render(w, "login.gohtml", model.ViewData{Title: "Login"})
	if err != nil {
		t.Fatalf("Render login.gohtml: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Iniciar") {
		t.Errorf("esperaba texto 'Iniciar' en el HTML, body: %q", body[:min(len(body), 200)])
	}
}

func TestRender_Registro(t *testing.T) {
	r, _ := tmpl.NewRenderer()
	w := httptest.NewRecorder()
	if err := r.Render(w, "registro.gohtml", model.ViewData{Title: "Registro"}); err != nil {
		t.Fatalf("Render registro.gohtml: %v", err)
	}
	if !strings.Contains(w.Body.String(), "cuenta") {
		t.Error("esperaba texto 'cuenta' en el HTML de registro")
	}
}

func TestRender_ListaEspacios(t *testing.T) {
	r, _ := tmpl.NewRenderer()
	w := httptest.NewRecorder()
	if err := r.Render(w, "lista_espacios.gohtml", model.ViewData{Title: "Espacios", Payload: nil}); err != nil {
		t.Fatalf("Render lista_espacios.gohtml: %v", err)
	}
	if !strings.Contains(w.Body.String(), "ReservaEspacios") {
		t.Error("esperaba texto 'ReservaEspacios' en el HTML")
	}
}

func TestRender_NuevoEspacio(t *testing.T) {
	r, _ := tmpl.NewRenderer()
	w := httptest.NewRecorder()
	if err := r.Render(w, "nuevo_espacio.gohtml", model.ViewData{Title: "Nuevo espacio"}); err != nil {
		t.Fatalf("Render nuevo_espacio.gohtml: %v", err)
	}
	if !strings.Contains(w.Body.String(), "espacio") {
		t.Error("esperaba texto 'espacio' en el HTML")
	}
}

func TestRender_DefaultTitle(t *testing.T) {
	r, _ := tmpl.NewRenderer()
	w := httptest.NewRecorder()
	// Title vacío → debe usar el título por defecto
	if err := r.Render(w, "login.gohtml", model.ViewData{}); err != nil {
		t.Fatalf("Render con título vacío: %v", err)
	}
	if !strings.Contains(w.Body.String(), "ReservaEspacios") {
		t.Error("esperaba el título por defecto 'ReservaEspacios'")
	}
}

func TestRender_WithFlashAndError(t *testing.T) {
	r, _ := tmpl.NewRenderer()
	w := httptest.NewRecorder()
	err := r.Render(w, "login.gohtml", model.ViewData{
		Title: "Login",
		Flash: "Operación exitosa",
		Error: "Credenciales inválidas",
	})
	if err != nil {
		t.Fatalf("Render con flash y error: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Operación exitosa") {
		t.Error("esperaba mensaje flash en el HTML")
	}
	if !strings.Contains(body, "Credenciales inválidas") {
		t.Error("esperaba mensaje de error en el HTML")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
