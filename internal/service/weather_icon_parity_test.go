package service

import (
	"strings"
	"testing"
)

func TestWeatherRetainsSaturnWMOFormattingIncludingFreezingAndSnow(t *testing.T) {
	for _, tc := range []struct {
		code int
		icon string
	}{
		{0, "🌤️"}, {1, "🌤️"}, {2, "🌥️"}, {3, "☁️"}, {45, "🌫️"}, {48, "😶‍🌫️"},
		{51, "🌦️"}, {53, "🌧️"}, {55, "🌧️"}, {56, "🌧️"}, {57, "🌧️"},
		{61, "🌦️"}, {63, "🌧️"}, {65, "🌧️"}, {66, "🌧️"}, {67, "🌧️"},
		{71, "❄️"}, {73, "❄️"}, {75, "❄️"}, {77, "❄️"}, {80, "🚿"}, {81, "🚿"},
		{82, "🚿"}, {85, "🚿"}, {86, "❄️"}, {95, "⛈️"}, {96, "⛈️"}, {99, "⛈️"}, {999, "unavailable"},
	} {
		w := weatherPayload{Timezone: "UTC"}
		w.Current.Time = "2026-09-11T12:00"
		w.Current.WeatherCode = &tc.code
		got, err := w.format("Chișinău, Moldova")
		if err != nil || !strings.Contains(got, "Precipitation: "+tc.icon+"\n") {
			t.Errorf("code=%d expected=%q err=%v output=%q", tc.code, tc.icon, err, got)
		}
	}
}
