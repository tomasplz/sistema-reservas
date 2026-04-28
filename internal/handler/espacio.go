package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"proyecto-monolito/internal/auth"
	"proyecto-monolito/internal/db"
	"proyecto-monolito/internal/model"
)

func (h *Handler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetCurrentUserID(r)
	spaces, err := h.Store.ListSpacesByUser(r.Context(), userID)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "No se pudieron cargar los espacios"}))
		return
	}
	h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Payload: spaces}))
}

func (h *Handler) NewSpaceForm(w http.ResponseWriter, r *http.Request) {
	h.Templates.Render(w, "nuevo_espacio.gohtml", h.view(r, model.ViewData{Title: "Nuevo espacio"}))
}

func (h *Handler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.Templates.Render(w, "nuevo_espacio.gohtml", h.view(r, model.ViewData{Title: "Nuevo espacio", Error: "No se pudo procesar el formulario"}))
		return
	}

	userID := auth.GetCurrentUserID(r)
	form, err := parseSpaceForm(r)
	if err != nil {
		h.Templates.Render(w, "nuevo_espacio.gohtml", h.view(r, model.ViewData{Title: "Nuevo espacio", Error: err.Error()}))
		return
	}

	_, err = h.Store.CreateSpace(r.Context(), userID, form.Nombre, form.Tipo, form.HoraApertura, form.HoraCierre, form.DuracionMinMinutos, form.PrecioHora, form.RecargoFinSemana, form.DescuentoVolumen, form.HorasParaDescuento)
	if err != nil {
		h.Templates.Render(w, "nuevo_espacio.gohtml", h.view(r, model.ViewData{Title: "Nuevo espacio", Error: "No se pudo crear el espacio"}))
		return
	}

	h.Auth.SetFlash(w, r, "Espacio creado con éxito")
	http.Redirect(w, r, "/espacios", http.StatusSeeOther)
}

func (h *Handler) SpaceDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "ID de espacio inválido"}))
		return
	}

	space, err := h.Store.GetSpaceByID(r.Context(), id)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "Espacio no encontrado"}))
		return
	}
	if space.UsuarioID != auth.GetCurrentUserID(r) {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "No autorizado"}))
		return
	}

	flash, _ := h.Auth.GetFlash(w, r)
	reservas, err := h.Store.ListReservasBySpace(r.Context(), id)
	if err != nil {
		h.Templates.Render(w, "detalle_espacio.gohtml", h.view(r, model.ViewData{
			Title: "Detalle de espacio", Flash: flash,
			Error: "No se pudieron cargar las reservas",
			Payload: struct {
				Space    db.Space
				Reservas []db.Reserva
			}{Space: space},
		}))
		return
	}

	h.Templates.Render(w, "detalle_espacio.gohtml", h.view(r, model.ViewData{
		Title: "Detalle de espacio", Flash: flash,
		Payload: struct {
			Space    db.Space
			Reservas []db.Reserva
		}{Space: space, Reservas: reservas},
	}))
}

func (h *Handler) EditSpaceForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "ID de espacio inválido"}))
		return
	}

	space, err := h.Store.GetSpaceByID(r.Context(), id)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "No se encontró el espacio"}))
		return
	}

	if space.UsuarioID != auth.GetCurrentUserID(r) {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "No autorizado"}))
		return
	}

	h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Payload: space}))
}

func (h *Handler) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.Templates.Render(w, "lista_espacios.gohtml", h.view(r, model.ViewData{Title: "Mis espacios", Error: "ID de espacio inválido"}))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Error: "No se pudo procesar el formulario"}))
		return
	}

	space, err := h.Store.GetSpaceByID(r.Context(), id)
	if err != nil {
		h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Error: "No se encontró el espacio"}))
		return
	}
	if space.UsuarioID != auth.GetCurrentUserID(r) {
		h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Error: "No autorizado"}))
		return
	}

	form, err := parseSpaceForm(r)
	if err != nil {
		h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Error: err.Error()}))
		return
	}

	if err := h.Store.UpdateSpace(r.Context(), form.Nombre, form.Tipo, form.HoraApertura, form.HoraCierre, form.DuracionMinMinutos, form.PrecioHora, form.RecargoFinSemana, form.DescuentoVolumen, form.HorasParaDescuento, id); err != nil {
		h.Templates.Render(w, "editar_espacio.gohtml", h.view(r, model.ViewData{Title: "Editar espacio", Error: "No se pudo actualizar el espacio"}))
		return
	}

	h.Auth.SetFlash(w, r, "Espacio actualizado con éxito")
	http.Redirect(w, r, "/espacios/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func parseSpaceForm(r *http.Request) (model.SpaceForm, error) {
	duracion, err := strconv.Atoi(r.PostForm.Get("duracion_min_minutos"))
	if err != nil {
		return model.SpaceForm{}, err
	}
	horasParaDescuento, err := strconv.Atoi(r.PostForm.Get("horas_para_descuento"))
	if err != nil {
		return model.SpaceForm{}, err
	}
	precioHora, err := strconv.ParseFloat(r.PostForm.Get("precio_hora"), 64)
	if err != nil {
		return model.SpaceForm{}, err
	}
	recargo, err := strconv.ParseFloat(r.PostForm.Get("recargo_fin_semana"), 64)
	if err != nil {
		return model.SpaceForm{}, err
	}
	descuento, err := strconv.ParseFloat(r.PostForm.Get("descuento_volumen"), 64)
	if err != nil {
		return model.SpaceForm{}, err
	}

	return model.SpaceForm{
		Nombre:             r.PostForm.Get("nombre"),
		Tipo:               r.PostForm.Get("tipo"),
		HoraApertura:       r.PostForm.Get("hora_apertura"),
		HoraCierre:         r.PostForm.Get("hora_cierre"),
		DuracionMinMinutos: int32(duracion),
		PrecioHora:         precioHora,
		RecargoFinSemana:   recargo,
		DescuentoVolumen:   descuento,
		HorasParaDescuento: int32(horasParaDescuento),
	}, nil
}
