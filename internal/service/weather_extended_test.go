package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Each fixture distinguishes yesterday's value from the selected provider day/hour.
var extendedWeatherMetrics = []struct {
	period, field, unit, label, want string
	value                            float64
}{
	{"daily", "temperature_2m_min", "°C", "Temperature min today", "-2.5 °C", -2.5},
	{"daily", "temperature_2m_max", "°C", "Temperature max today", "21.5 °C", 21.5},
	{"daily", "apparent_temperature_min", "°C", "Feels min today", "-4 °C", -4},
	{"daily", "apparent_temperature_max", "°C", "Feels max today", "23 °C", 23},
	{"daily", "precipitation_probability_max", "%", "Precip. chance max today", "75 %", 75},
	{"daily", "precipitation_sum", "mm", "Precipitation total today", "3.2 mm", 3.2},
	{"daily", "snowfall_sum", "cm", "Snowfall total today", "0 cm", 0},
	{"daily", "daylight_duration", "s", "Daylight today", "12h 47m", 46079.9},
	{"daily", "sunshine_duration", "s", "Sunshine today", "8h 03m", 28980},
	{"hourly", "wind_direction_10m", "°", "Wind direction (hourly)", "270 °", 270},
	{"hourly", "wind_gusts_10m", "km/h", "Wind gusts (prev hour max)", "35.2 km/h", 35.2},
	{"hourly", "cloud_cover", "%", "Cloud cover (hourly)", "42 %", 42},
	{"hourly", "visibility", "m", "Visibility (hourly)", "12000 m", 12000},
	{"hourly", "dew_point_2m", "°C", "Dew point (hourly)", "-1.5 °C", -1.5},
}

func extendedWeatherFixture() map[string]any {
	f := map[string]any{
		"timezone":        "Europe/Chisinau",
		"current_weather": map[string]any{"time": "2026-09-11T12:15"},
		"daily":           map[string]any{"time": []string{"2026-09-10", "2026-09-11"}},
		"hourly":          map[string]any{"time": []string{"2026-09-10T12:00", "2026-09-11T12:00"}},
		"daily_units":     map[string]any{}, "hourly_units": map[string]any{},
	}
	for _, m := range extendedWeatherMetrics {
		f[m.period].(map[string]any)[m.field] = []any{999, m.value}
		f[m.period+"_units"].(map[string]any)[m.field] = m.unit
	}
	return f
}

func formatExtendedFixture(t *testing.T, f map[string]any) string {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var w weatherPayload
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	got, err := w.format("Chișinău, Moldova")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWeatherExtendedMetricsUseProviderAxesAndUnits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/geo" {
			_, _ = w.Write([]byte(`{"geonames":[{"name":"Chișinău","countryName":"Moldova","lat":"47","lng":"28"}]}`))
			return
		}
		for _, m := range extendedWeatherMetrics {
			if !strings.Contains(","+r.URL.Query().Get(m.period)+",", ","+m.field+",") {
				t.Errorf("missing %s query field %s", m.period, m.field)
			}
		}
		_ = json.NewEncoder(w).Encode(extendedWeatherFixture())
	}))
	defer srv.Close()
	got, err := (&WeatherService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", ForecastURL: srv.URL + "/forecast"}).Get(context.Background(), "chisinau")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range extendedWeatherMetrics {
		if !strings.Contains(got, m.label+": "+m.want+"\n") {
			t.Errorf("missing %s: %s", m.label, m.want)
		}
	}
}

func TestWeatherExtendedMissingMetricsAreUnavailable(t *testing.T) {
	for _, mode := range []string{"omitted", "null", "short", "unmatched axis"} {
		t.Run(mode, func(t *testing.T) {
			f := extendedWeatherFixture()
			for _, m := range extendedWeatherMetrics {
				values := f[m.period].(map[string]any)
				switch mode {
				case "omitted":
					delete(values, m.field)
				case "null":
					values[m.field] = []any{999, nil}
				case "short":
					values[m.field] = []any{999}
				case "unmatched axis":
					f["current_weather"] = map[string]any{"time": "2026-09-12T12:15"}
				}
			}
			got := formatExtendedFixture(t, f)
			for _, m := range extendedWeatherMetrics {
				if !strings.Contains(got, m.label+": unavailable\n") {
					t.Errorf("%s fabricated or absent in %s", m.label, got)
				}
			}
		})
	}
}

func TestWeatherDailyDurationFormatting(t *testing.T) {
	for _, tc := range []struct {
		value      any
		unit, want string
	}{
		{0, "s", "0h 00m"}, {59.9, "s", "0h 00m"}, {3600, "s", "1h 00m"},
		{86400, "s", "24h 00m"}, {-1, "s", "unavailable"}, {nil, "s", "unavailable"},
		{json.Number("1e999"), "s", "unavailable"}, {60, "hours", "unavailable"}, {60, "", "unavailable"},
	} {
		f := extendedWeatherFixture()
		f["daily"].(map[string]any)["daylight_duration"] = []any{999, tc.value}
		f["daily_units"].(map[string]any)["daylight_duration"] = tc.unit
		got := formatExtendedFixture(t, f)
		if !strings.Contains(got, "Daylight today: "+tc.want+"\n") {
			t.Errorf("seconds=%v unit=%q want=%s got=%s", tc.value, tc.unit, tc.want, got)
		}
	}
}
