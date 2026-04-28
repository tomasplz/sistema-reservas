package logic

import (
	"context"
	"testing"

	"proyecto-monolito/internal/db"
)

type fakeReservationStore struct {
	reservas []db.Reserva
}

func (f fakeReservationStore) GetReservasBySpaceAndDate(ctx context.Context, espacioID int64, fecha string) ([]db.Reserva, error) {
	var result []db.Reserva
	for _, reserva := range f.reservas {
		if reserva.EspacioID == espacioID && reserva.Fecha == fecha {
			result = append(result, reserva)
		}
	}
	return result, nil
}

func TestCalcularPrecio(t *testing.T) {
	sample := db.Space{
		PrecioHora:         100,
		RecargoFinSemana:   0.25,
		DescuentoVolumen:   0.20,
		HorasParaDescuento: 4,
	}

	cases := []struct {
		name       string
		fecha      string
		horaInicio string
		horaFin    string
		expected   float64
	}{
		{"Dia laboral sin descuento", "2026-04-30", "10:00", "12:00", 200.00},
		{"Fin de semana sin descuento", "2026-05-03", "10:00", "12:00", 250.00},
		{"Descuento de volumen", "2026-04-30", "09:00", "14:00", 400.00},
		{"Fin de semana con descuento", "2026-05-03", "08:00", "13:00", 500.00},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			price := CalcularPrecio(sample, tc.fecha, tc.horaInicio, tc.horaFin)
			if price != tc.expected {
				t.Fatalf("expected %.2f, got %.2f", tc.expected, price)
			}
		})
	}
}

func TestValidarDisponibilidad(t *testing.T) {
	espacio := db.Space{
		ID:                 1,
		HoraApertura:       "08:00",
		HoraCierre:         "18:00",
		DuracionMinMinutos: 60,
	}

	store := fakeReservationStore{reservas: []db.Reserva{{EspacioID: 1, Fecha: "2026-05-01", HoraInicio: "10:00", HoraFin: "12:00"}}}

	cases := []struct {
		name       string
		fecha      string
		horaInicio string
		horaFin    string
		wantOK     bool
	}{
		{"Disponible horario libre", "2026-05-01", "12:00", "13:00", true},
		{"Fuera de horario", "2026-05-01", "07:00", "09:00", false},
		{"Solapamiento", "2026-05-01", "11:00", "13:00", false},
		{"Duracion menor al minimo", "2026-05-01", "12:00", "12:30", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := ValidarDisponibilidad(context.Background(), espacio, tc.fecha, tc.horaInicio, tc.horaFin, store)
			if ok != tc.wantOK {
				if tc.wantOK {
					t.Fatalf("expected available but got error %v", err)
				} else if err == nil {
					t.Fatal("expected error but got none")
				}
			}
		})
	}
}
