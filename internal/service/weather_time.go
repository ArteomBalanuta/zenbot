package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const metricUnavailable = "unavailable"

type WeatherService struct {
	HTTP        *http.Client
	GeoURL      string
	ForecastURL string
	// Now is retained for source compatibility. Forecast dates come from the
	// provider's local time axis, rather than the process clock.
	Now func() time.Time
}

type geocodedLocation struct {
	Name    string `json:"name"`
	Country string `json:"countryName"`
	Lat     string `json:"lat"`
	Lng     string `json:"lng"`
}

func utilityHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func utilityURL(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid provider URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("provider URL must be absolute HTTP or HTTPS")
	}
	return u, nil
}

func geocode(ctx context.Context, client *http.Client, endpoint, location string) (geocodedLocation, error) {
	if strings.TrimSpace(location) == "" {
		return geocodedLocation{}, fmt.Errorf("location is required")
	}
	if endpoint == "" {
		endpoint = "https://secure.geonames.org/searchJSON"
	}
	u, err := utilityURL(endpoint)
	if err != nil {
		return geocodedLocation{}, err
	}
	q := u.Query()
	q.Set("q", location)
	q.Set("maxRows", "1")
	q.Set("type", "json")
	if q.Get("username") == "" {
		q.Set("username", "dev1")
	}
	u.RawQuery = q.Encode()
	var payload struct {
		Results []geocodedLocation `json:"geonames"`
		Status  *struct {
			Message string `json:"message"`
			Value   int    `json:"value"`
		} `json:"status"`
	}
	if err := getJSON(ctx, client, u.String(), &payload); err != nil {
		return geocodedLocation{}, err
	}
	if payload.Status != nil {
		return geocodedLocation{}, fmt.Errorf("geocoder provider failed (status %d)", payload.Status.Value)
	}
	if len(payload.Results) == 0 {
		return geocodedLocation{}, fmt.Errorf("location not found")
	}
	r := payload.Results[0]
	for _, c := range []struct {
		value string
		limit float64
	}{{r.Lat, 90}, {r.Lng, 180}} {
		v, err := strconv.ParseFloat(c.value, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > c.limit {
			return geocodedLocation{}, fmt.Errorf("geocoder returned invalid coordinates")
		}
	}
	return r, nil
}

func (s *WeatherService) Get(ctx context.Context, location string) (string, error) {
	client := utilityHTTPClient(s.HTTP)
	r, err := geocode(ctx, client, s.GeoURL, location)
	if err != nil {
		return "", err
	}
	endpoint := s.ForecastURL
	if endpoint == "" {
		endpoint = "https://api.open-meteo.com/v1/forecast"
	}
	f, err := utilityURL(endpoint)
	if err != nil {
		return "", err
	}
	q := f.Query()
	q.Set("latitude", r.Lat)
	q.Set("longitude", r.Lng)
	q.Set("current_weather", "true")
	q.Set("daily", "sunrise,sunset,shortwave_radiation_sum,uv_index_max")
	q.Set("hourly", "pressure_msl,surface_pressure,soil_temperature_18cm,soil_moisture_3_to_9cm,diffuse_radiation,shortwave_radiation,apparent_temperature,relative_humidity_2m")
	q.Set("timezone", "auto")
	q.Set("forecast_days", "1")
	f.RawQuery = q.Encode()
	var forecast weatherPayload
	if err := getJSON(ctx, client, f.String(), &forecast); err != nil {
		return "", err
	}
	if forecast.Error {
		return "", fmt.Errorf("weather provider rejected the request")
	}
	name := r.Name
	if name == "" {
		name = location
	}
	return forecast.format(strings.Trim(strings.Join([]string{name, r.Country}, ", "), ", "))
}

type weatherPayload struct {
	Error    bool   `json:"error"`
	Timezone string `json:"timezone"`
	Current  struct {
		Temperature *json.Number `json:"temperature"`
		Windspeed   *json.Number `json:"windspeed"`
		WeatherCode *int         `json:"weathercode"`
		Time        string       `json:"time"`
	} `json:"current_weather"`
	CurrentUnits struct {
		Temperature string `json:"temperature"`
		Windspeed   string `json:"windspeed"`
	} `json:"current_weather_units"`
	DailyRaw struct {
		Time      []string       `json:"time"`
		Sunrise   []string       `json:"sunrise"`
		Sunset    []string       `json:"sunset"`
		UV        []*json.Number `json:"uv_index_max"`
		Radiation []*json.Number `json:"shortwave_radiation_sum"`
	} `json:"daily"`
	DailyUnitsRaw struct {
		UV        string `json:"uv_index_max"`
		Radiation string `json:"shortwave_radiation_sum"`
	} `json:"daily_units"`
	HourlyRaw struct {
		Time      []string       `json:"time"`
		Apparent  []*json.Number `json:"apparent_temperature"`
		Humidity  []*json.Number `json:"relative_humidity_2m"`
		Surface   []*json.Number `json:"surface_pressure"`
		Sea       []*json.Number `json:"pressure_msl"`
		Shortwave []*json.Number `json:"shortwave_radiation"`
		Diffuse   []*json.Number `json:"diffuse_radiation"`
		SoilTemp  []*json.Number `json:"soil_temperature_18cm"`
		SoilMoist []*json.Number `json:"soil_moisture_3_to_9cm"`
	} `json:"hourly"`
	HourlyUnitsRaw struct {
		Apparent  string `json:"apparent_temperature"`
		Humidity  string `json:"relative_humidity_2m"`
		Surface   string `json:"surface_pressure"`
		Sea       string `json:"pressure_msl"`
		Shortwave string `json:"shortwave_radiation"`
		Diffuse   string `json:"diffuse_radiation"`
		SoilTemp  string `json:"soil_temperature_18cm"`
		SoilMoist string `json:"soil_moisture_3_to_9cm"`
	} `json:"hourly_units"`
}

func providerLocation(zone string) (*time.Location, error) {
	if strings.TrimSpace(zone) == "" {
		return nil, fmt.Errorf("provider timezone is missing")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("invalid provider timezone: %w", err)
	}
	return loc, nil
}

func providerTime(value string, loc *time.Location) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.In(loc), nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid provider timestamp")
}

func numberMetric(value *json.Number, unit string) string {
	if value == nil || value.String() == "" {
		return metricUnavailable
	}
	return strings.TrimSpace(value.String() + " " + unit)
}

func metricAt(values []*json.Number, index int, unit string) string {
	if index < 0 || index >= len(values) {
		return metricUnavailable
	}
	return numberMetric(values[index], unit)
}

func (w weatherPayload) format(area string) (string, error) {
	loc, err := providerLocation(w.Timezone)
	if err != nil {
		return "", err
	}
	current, err := providerTime(w.Current.Time, loc)
	if err != nil {
		return "", err
	}
	hour, day := -1, -1
	for i, value := range w.HourlyRaw.Time {
		at, err := providerTime(value, loc)
		if err != nil {
			return "", fmt.Errorf("hourly axis: %w", err)
		}
		if !at.After(current) && current.Sub(at) < time.Hour {
			hour = i
		}
	}
	for i, value := range w.DailyRaw.Time {
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return "", fmt.Errorf("invalid daily date")
		}
		if value == current.Format("2006-01-02") {
			day = i
		}
	}
	solar := func(values []string) (string, error) {
		if day < 0 || day >= len(values) || values[day] == "" {
			return metricUnavailable, nil
		}
		at, err := providerTime(values[day], loc)
		if err != nil {
			return "", err
		}
		return at.Format("Mon, 02 Jan 2006 15-04-05 -0700"), nil
	}
	rise, err := solar(w.DailyRaw.Sunrise)
	if err != nil {
		return "", fmt.Errorf("sunrise: %w", err)
	}
	set, err := solar(w.DailyRaw.Sunset)
	if err != nil {
		return "", fmt.Errorf("sunset: %w", err)
	}
	code := metricUnavailable
	if w.Current.WeatherCode != nil {
		icons := map[int]string{0: "☀️", 1: "🌤️", 2: "⛅", 3: "☁️", 45: "🌫️", 48: "🌫️", 51: "🌦️", 53: "🌦️", 55: "🌧️", 61: "🌧️", 63: "🌧️", 65: "🌧️", 71: "🌨️", 73: "🌨️", 75: "❄️", 80: "🌦️", 81: "🌧️", 82: "🌧️", 95: "⛈️", 96: "⛈️", 99: "⛈️"}
		if icon, ok := icons[*w.Current.WeatherCode]; ok {
			code = icon
		}
	}
	lines := []string{
		"Weather forecast for today: **" + area + "**",
		"Temperature: " + numberMetric(w.Current.Temperature, w.CurrentUnits.Temperature),
		"Feels temp: " + metricAt(w.HourlyRaw.Apparent, hour, w.HourlyUnitsRaw.Apparent),
		"Air Humidity: " + metricAt(w.HourlyRaw.Humidity, hour, w.HourlyUnitsRaw.Humidity),
		"Precipitation: " + code,
		"Wind speed: " + numberMetric(w.Current.Windspeed, w.CurrentUnits.Windspeed),
		"Pressure surface: " + metricAt(w.HourlyRaw.Surface, hour, w.HourlyUnitsRaw.Surface),
		"Pressure sea level: " + metricAt(w.HourlyRaw.Sea, hour, w.HourlyUnitsRaw.Sea),
		"UV day max index: " + metricAt(w.DailyRaw.UV, day, w.DailyUnitsRaw.UV),
		"Short wave radiation day sum: " + metricAt(w.DailyRaw.Radiation, day, w.DailyUnitsRaw.Radiation),
		"ShortWave rad: " + metricAt(w.HourlyRaw.Shortwave, hour, w.HourlyUnitsRaw.Shortwave),
		"Diffuse rad: " + metricAt(w.HourlyRaw.Diffuse, hour, w.HourlyUnitsRaw.Diffuse),
		"Time: " + current.Format("Mon, 02 Jan 2006 15-04-05 -0700"),
		"Sun rise: " + rise, "Sun set: " + set,
		"Soil temp 18cm: " + metricAt(w.HourlyRaw.SoilTemp, hour, w.HourlyUnitsRaw.SoilTemp),
		"Soil moist 3-9cm: " + metricAt(w.HourlyRaw.SoilMoist, hour, w.HourlyUnitsRaw.SoilMoist),
	}
	return alignLiteralLines(lines, true), nil
}

type TimeService struct {
	HTTP                            *http.Client
	GeoURL, SunriseURL, TimezoneURL string
}

type solarResults struct {
	Date      string `json:"date"`
	Timezone  string `json:"timezone"`
	Sunrise   string `json:"sunrise"`
	Sunset    string `json:"sunset"`
	First     string `json:"first_light"`
	Last      string `json:"last_light"`
	Dawn      string `json:"dawn"`
	Dusk      string `json:"dusk"`
	Noon      string `json:"solar_noon"`
	Golden    string `json:"golden_hour"`
	Length    string `json:"day_length"`
	UTCOffset *int   `json:"utc_offset"`
}

func solarClock(value string) (string, error) {
	if value == "" {
		return metricUnavailable, nil
	}
	for _, layout := range []string{"3:04:05 PM", "3:04 PM", "15:04:05", "15:04"} {
		if _, err := time.Parse(layout, value); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid solar timestamp")
}

func solarDuration(value string) (string, error) {
	if value == "" {
		return metricUnavailable, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid day length")
	}
	seconds := 0
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || (i == 0 && n > 24) || (i > 0 && n > 59) {
			return "", fmt.Errorf("invalid day length")
		}
		seconds = seconds*60 + n
	}
	if seconds > 24*60*60 {
		return "", fmt.Errorf("invalid day length")
	}
	return value, nil
}

func solarOffsetMatchesDate(current time.Time, minutes int) bool {
	if minutes < -24*60 || minutes > 24*60 {
		return false
	}
	// Under the candidate offset, this UTC interval maps exactly to the requested
	// civil date. Accept only if a real zone segment with that offset intersects
	// the interval. Constructing midnight in the local zone could normalize to
	// the previous date when a transition skips midnight.
	start := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).Add(-time.Duration(minutes) * time.Minute)
	end := start.Add(24 * time.Hour)
	for at := start.In(current.Location()); at.Before(end); {
		_, offset := at.Zone()
		if offset%60 == 0 && offset/60 == minutes {
			return true
		}
		_, next := at.ZoneBounds()
		if next.IsZero() || !next.After(at) {
			break
		}
		at = next
	}
	return false
}

func (s *TimeService) Get(ctx context.Context, location string) (string, error) {
	client := utilityHTTPClient(s.HTTP)
	r, err := geocode(ctx, client, s.GeoURL, location)
	if err != nil {
		return "", err
	}
	tz := s.TimezoneURL
	if tz == "" {
		tz = "https://timeapi.io/api/Time/current/coordinate?latitude=%s&longitude=%s"
	}
	tzURL, err := utilityURL(fmt.Sprintf(tz, url.QueryEscape(r.Lat), url.QueryEscape(r.Lng)))
	if err != nil {
		return "", err
	}
	var tr struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	}
	if err := getJSON(ctx, client, tzURL.String(), &tr); err != nil {
		return "", err
	}
	loc, err := providerLocation(tr.TimeZone)
	if err != nil {
		return "", err
	}
	current, err := providerTime(tr.DateTime, loc)
	if err != nil {
		return "", err
	}
	// Freeze the solar query to this clock snapshot, including across midnight.
	date := current.Format("2006-01-02")
	sun := s.SunriseURL
	if sun == "" {
		sun = "https://api.sunrisesunset.io/json?lat=%s&lng=%s"
	}
	sunURL, err := utilityURL(fmt.Sprintf(sun, url.QueryEscape(r.Lat), url.QueryEscape(r.Lng)))
	if err != nil {
		return "", err
	}
	query := sunURL.Query()
	query.Set("date", date)
	query.Set("timezone", tr.TimeZone)
	sunURL.RawQuery = query.Encode()
	var sr struct {
		Status  string        `json:"status"`
		TZID    string        `json:"tzid"`
		Results *solarResults `json:"results"`
	}
	if err := getJSON(ctx, client, sunURL.String(), &sr); err != nil {
		return "", err
	}
	if (sr.Status != "" && sr.Status != "OK") || sr.Results == nil {
		return "", fmt.Errorf("solar provider failed")
	}
	if sr.Results.Date != date || sr.Results.Timezone != tr.TimeZone || (sr.TZID != "" && sr.TZID != tr.TimeZone) {
		return "", fmt.Errorf("solar date or timezone disagrees with current clock")
	}
	_, seconds := current.Zone()
	if seconds%60 != 0 {
		return "", fmt.Errorf("current UTC offset is not in whole minutes")
	}
	if sr.Results.UTCOffset != nil && !solarOffsetMatchesDate(current, *sr.Results.UTCOffset) {
		return "", fmt.Errorf("solar UTC offset disagrees with requested date and timezone")
	}
	minutes := seconds / 60
	sign := "+"
	if minutes < 0 {
		sign = "-"
		minutes = -minutes
	}
	offset := fmt.Sprintf("%s%02d:%02d", sign, minutes/60, minutes%60)
	lines := []string{"today: " + date, "time: " + current.Format(time.RFC1123), "zone: " + tr.TimeZone, "UTC offset: " + offset}
	for _, field := range []struct{ label, value string }{{"sun rise", sr.Results.Sunrise}, {"sun set", sr.Results.Sunset}, {"first light", sr.Results.First}, {"last light", sr.Results.Last}, {"dawn", sr.Results.Dawn}, {"dusk", sr.Results.Dusk}, {"solar noon", sr.Results.Noon}, {"golden hour", sr.Results.Golden}} {
		value, err := solarClock(field.value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", field.label, err)
		}
		lines = append(lines, field.label+": "+value)
	}
	length, err := solarDuration(sr.Results.Length)
	if err != nil {
		return "", err
	}
	lines = append(lines, "day length: "+length)
	return fmt.Sprintf("\\n Time: **%s, %s** \\n ", location, r.Country) + alignLiteralLines(lines, false), nil
}
