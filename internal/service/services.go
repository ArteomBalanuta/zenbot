package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

// CommandOutput is the only output seam used by migrated command services.
type CommandOutput interface {
	Chat(author, text string, whisper bool) error
	Raw(payload any) error
}

// Bundle contains the non-agent services attached to an engine.
type Bundle struct {
	Security *SecurityService
	Mail     *MailService
	Notes    *NoteService
	Users    *UserService
	Ping     *PingService
	Weather  *WeatherService
	Time     *TimeService
	Search   *SearchService
	SCP      *SCPService
	DBZ      *DBZService
}

type UserService struct {
	Queries  repository.UserQueryRepository
	Identity repository.IdentityRepository
	GroupB   repository.SqlUtilGroupBRepository
}

func (s *UserService) RegisteredUsers(ctx context.Context) ([]repository.RegisteredUser, error) {
	return s.Queries.RegisteredUsers(ctx)
}

func (s *UserService) NicksByTrip(ctx context.Context, trip string) ([]string, error) {
	return s.Queries.NicksByTrip(ctx, trip)
}
func (s *UserService) BasicUserData(ctx context.Context, hash, trip string) (string, error) {
	return s.Queries.BasicUserData(ctx, hash, trip)
}
func (s *UserService) IsNameRegistered(name string) (bool, error) {
	return s.Identity.IsNameRegistered(name)
}
func (s *UserService) IsTripRegistered(trip string) (bool, error) {
	return s.Identity.IsTripRegistered(trip)
}
func (s *UserService) Register(name, trip string, role model.Role) error {
	return s.Identity.Register(name, trip, role)
}
func (s *UserService) RegisterNameByTrip(name, trip string) error {
	return s.Identity.RegisterNameByTrip(name, trip)
}
func (s *UserService) RegisterTripByName(name, trip string) error {
	return s.Identity.RegisterTripByName(name, trip)
}
func (s *UserService) LastMessages(name, trip string, count int) ([]model.Message, error) {
	return s.Identity.LastMessages(name, trip, count)
}

func (s *UserService) DeleteIdentity(ctx context.Context, nameOrTrip string) (repository.DeleteResult, error) {
	capability, ok := s.GroupB.(repository.SaturnAuthorizedDeleteRepository)
	if !ok {
		return repository.DeleteResult{}, fmt.Errorf("authorized Group B delete unavailable")
	}
	return capability.DeleteIdentityAuthorized(ctx, nameOrTrip)
}

func (s *UserService) SaturnRegisteredUsers(ctx context.Context) ([]repository.SaturnRegisteredUser, error) {
	if s.GroupB == nil {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnRegisteredUsers(ctx)
}

// SaturnLastMessages exposes the Saturn-shaped compatibility read without
// changing the existing Zenbot history contract.
func (s *UserService) SaturnLastMessages(ctx context.Context, name *string, trip string, count int) ([]repository.SaturnLastMessage, error) {
	if s.GroupB == nil {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnLastMessages(ctx, name, trip, count)
}

type MailService struct {
	DB     *sql.DB
	Out    CommandOutput
	GroupB repository.SqlUtilGroupBRepository
}

func (s *MailService) Queue(message, owner, receiver string, whisper bool) error {
	_, err := s.QueueResolved(message, owner, receiver, whisper)
	return err
}

// QueueResolved persists pending mail and returns the resolved recipient trips
// used by Saturn's scheduling acknowledgement.
func (s *MailService) QueueResolved(message, owner, receiver string, whisper bool) (string, error) {
	receiver = strings.TrimPrefix(strings.TrimSpace(receiver), "@")
	if receiver == "" {
		return "", fmt.Errorf("receiver cannot be blank")
	}
	rows, e := s.DB.Query(`SELECT t.trip FROM trip_names tn INNER JOIN names n ON tn.name_id=n.id INNER JOIN trips t ON tn.trip_id=t.id WHERE LOWER(n.name)=$1 OR LOWER(t.trip)=$2`, strings.ToLower(receiver), strings.ToLower(receiver))
	if e != nil {
		return "", e
	}
	var trips []string
	for rows.Next() {
		var trip string
		if e = rows.Scan(&trip); e != nil {
			rows.Close()
			return "", e
		}
		trips = append(trips, trip)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return "", e
	}
	rows.Close()
	if len(trips) == 0 {
		return "", fmt.Errorf("user not registered")
	}
	receivers := strings.Join(trips, ",")
	if message != "" {
		message += " "
	}
	escapedMessage, _ := json.Marshal(message)
	message = string(escapedMessage[1 : len(escapedMessage)-1])
	// Saturn logs delivery-write failures but still acknowledges scheduling.
	_, _ = s.DB.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES($1,$2,$3,'PENDING',$4,$5)`, owner, receivers, message, time.Now().UnixMilli(), strconv.FormatBool(whisper))
	return receivers, nil
}
func (s *MailService) RegisteredUsers() string {
	rows, e := s.DB.Query(`SELECT DISTINCT n.name,t.trip FROM trip_names tn INNER JOIN trips t ON tn.trip_id=t.id INNER JOIN names n ON tn.name_id=n.id ORDER BY t.trip DESC`)
	if e != nil {
		return ""
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var name, trip string
		if rows.Scan(&name, &trip) == nil {
			b.WriteString(name)
			b.WriteByte(' ')
			b.WriteString(trip)
			b.WriteString("\\n")
		}
	}
	return b.String()
}

// SaturnRegisteredUsers exposes the Saturn-shaped compatibility read without
// changing the existing formatted directory contract.
func (s *MailService) SaturnRegisteredUsers(ctx context.Context) ([]repository.SaturnRegisteredUser, error) {
	if s.GroupB == nil {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnRegisteredUsers(ctx)
}

func (s *MailService) Pending(receiver, trip string) ([]model.Mail, error) {
	rows, e := s.DB.Query(`SELECT id,owner,receiver,message,status,created_on,is_whisper FROM mail WHERE status='PENDING' AND (LOCATE(',' || LOWER($1) || ',', ',' || LOWER(receiver) || ',') > 0 OR LOCATE(',' || LOWER($2) || ',', ',' || LOWER(receiver) || ',') > 0) ORDER BY id`, receiver, trip)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Mail
	for rows.Next() {
		var m model.Mail
		var w string
		if e = rows.Scan(&m.ID, &m.Owner, &m.Receiver, &m.Message, &m.Status, &m.CreatedOn, &w); e != nil {
			return nil, e
		}
		m.IsWhisper = strings.EqualFold(w, "true")
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *MailService) MarkDelivered(id int64) error {
	_, e := s.DB.Exec(`UPDATE mail SET status='DELIVERED' WHERE id=$1`, id)
	return e
}

type NoteService struct {
	DB  *sql.DB
	Out CommandOutput
}

func (s *NoteService) Save(trip, note string) error {
	_, e := s.DB.Exec(`INSERT INTO notes(trip,note,created_on) VALUES($1,$2,$3)`, trip, note, time.Now().UnixMilli())
	return e
}
func (s *NoteService) List(trip string) ([]string, error) {
	rows, e := s.DB.Query(`SELECT note FROM notes WHERE trip=$1 ORDER BY id`, trip)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []string
	for rows.Next() {
		var n string
		if e = rows.Scan(&n); e != nil {
			return nil, e
		}
		b, _ := json.Marshal(n)
		o = append(o, string(b[1:len(b)-1]))
	}
	return o, rows.Err()
}
func (s *NoteService) Clear(trip string) error {
	_, e := s.DB.Exec(`DELETE FROM notes WHERE trip=$1`, trip)
	return e
}

type PingService struct {
	HTTP    *http.Client
	Address string
}

func (s *PingService) Ping(ctx context.Context) (time.Duration, error) {
	a := s.Address
	if a == "" {
		a = "hack.chat:80"
	}
	st := time.Now()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	c, e := dialer.DialContext(ctx, "tcp", a)
	if e != nil {
		return 0, e
	}
	_ = c.Close()
	return time.Since(st), nil
}

type WeatherService struct {
	HTTP        *http.Client
	GeoURL      string
	ForecastURL string
	Now         func() time.Time
}

func (s *WeatherService) Get(ctx context.Context, location string) (string, error) {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if s.GeoURL == "" {
		s.GeoURL = "http://api.geonames.org/search"
	}
	u, _ := url.Parse(s.GeoURL)
	q := u.Query()
	q.Set("q", location)
	q.Set("maxRows", "1")
	q.Set("username", "dev1")
	u.RawQuery = q.Encode()
	var g struct {
		Results []struct {
			Name    string `json:"name"`
			Country string `json:"countryName"`
			Lat     string `json:"lat"`
			Lng     string `json:"lng"`
		} `json:"geonames"`
	}
	if e := getJSON(ctx, s.HTTP, u.String(), &g); e != nil {
		return "", e
	}
	if len(g.Results) == 0 {
		return "", fmt.Errorf("location not found")
	}
	r := g.Results[0]
	date := time.Now()
	if s.Now != nil {
		date = s.Now()
	}
	dateText := date.Format("2006-01-02")
	ep := s.ForecastURL
	if ep == "" {
		ep = "https://api.open-meteo.com/v1/forecast"
	}
	f, _ := url.Parse(ep)
	fq := f.Query()
	fq.Set("latitude", r.Lat)
	fq.Set("longitude", r.Lng)
	fq.Set("current_weather", "true")
	fq.Set("daily", "temperature_2m_max,temperature_2m_min,precipitation_sum,sunrise,sunset,winddirection_10m_dominant,shortwave_radiation_sum,uv_index_max,uv_index_clear_sky_max,weather_code")
	fq.Set("hourly", "pressure_msl,surface_pressure,soil_temperature_18cm,soil_moisture_3_to_9cm,visibility,diffuse_radiation,shortwave_radiation,apparent_temperature,relative_humidity_2m")
	fq.Set("timezone", "auto")
	fq.Set("start_date", dateText)
	fq.Set("end_date", dateText)
	f.RawQuery = fq.Encode()
	var forecast weatherPayload
	if e := getJSON(ctx, s.HTTP, f.String(), &forecast); e != nil {
		return "", e
	}
	return forecast.format(r.Name + ", " + r.Country), nil
}

type weatherPayload struct {
	Timezone string `json:"timezone"`
	Current  struct {
		Temperature json.Number `json:"temperature"`
		Windspeed   json.Number `json:"windspeed"`
		WeatherCode int         `json:"weathercode"`
		Time        string      `json:"time"`
	} `json:"current_weather"`
	CurrentUnits struct {
		Temperature string `json:"temperature"`
		Windspeed   string `json:"windspeed"`
	} `json:"current_weather_units"`
	Daily    struct{ Sunrise, Sunset, UV, Radiation []string }                                            `json:"-"`
	Hourly   struct{ Apparent, Humidity, Surface, Sea, Shortwave, Diffuse, SoilTemp, SoilMoist []string } `json:"-"`
	DailyRaw struct {
		Sunrise   []string `json:"sunrise"`
		Sunset    []string `json:"sunset"`
		UV        []string `json:"uv_index_max"`
		Radiation []string `json:"shortwave_radiation_sum"`
	} `json:"daily"`
	DailyUnitsRaw struct {
		UV        string `json:"uv_index_max"`
		Radiation string `json:"shortwave_radiation_sum"`
	} `json:"daily_units"`
	HourlyRaw struct {
		Apparent  []string `json:"apparent_temperature"`
		Humidity  []string `json:"relative_humidity_2m"`
		Surface   []string `json:"surface_pressure"`
		Sea       []string `json:"pressure_msl"`
		Shortwave []string `json:"shortwave_radiation"`
		Diffuse   []string `json:"diffuse_radiation"`
		SoilTemp  []string `json:"soil_temperature_18cm"`
		SoilMoist []string `json:"soil_moisture_3_to_9cm"`
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

func (w weatherPayload) format(area string) string {
	// Saturn uses the local forecast hour and preserves its literal separators.
	loc, _ := time.LoadLocation(w.Timezone)
	now, _ := time.ParseInLocation("2006-01-02T15:04", w.Current.Time, loc)
	h := now.Hour()
	pick := func(v []string) string {
		if h >= 0 && h < len(v) {
			return v[h]
		}
		return ""
	}
	code := map[int]string{0: "☀️", 1: "🌤️", 2: "⛅", 3: "☁️", 45: "🌫️", 48: "🌫️", 51: "🌦️", 53: "🌦️", 55: "🌧️", 61: "🌧️", 63: "🌧️", 65: "🌧️", 71: "🌨️", 73: "🌨️", 75: "❄️", 80: "🌦️", 81: "🌧️", 82: "🌧️", 95: "⛈️", 96: "⛈️", 99: "⛈️"}[w.Current.WeatherCode]
	rfc := func(v string) string {
		t, _ := time.ParseInLocation("2006-01-02T15:04", v, loc)
		return t.Format("Mon, 02 Jan 2006 15-04-05 -0700")
	}
	lines := []string{fmt.Sprintf("Weather forecast for today: **%s**", area), fmt.Sprintf("Temperature: %s %s", w.Current.Temperature, w.CurrentUnits.Temperature), fmt.Sprintf("Feels temp: %s %s", pick(w.HourlyRaw.Apparent), w.HourlyUnitsRaw.Apparent), fmt.Sprintf("Air Humidity: %s %s", pick(w.HourlyRaw.Humidity), w.HourlyUnitsRaw.Humidity), "Precipitation: " + code, fmt.Sprintf("Wind speed: %s %s", w.Current.Windspeed, w.CurrentUnits.Windspeed), fmt.Sprintf("Pressure surface: %s %s", pick(w.HourlyRaw.Surface), w.HourlyUnitsRaw.Surface), fmt.Sprintf("Pressure sea level: %s %s", pick(w.HourlyRaw.Sea), w.HourlyUnitsRaw.Sea), "\u2009\u2009\u2009 ", fmt.Sprintf("UV day max index: %s %s", first(w.DailyRaw.UV), w.DailyUnitsRaw.UV), fmt.Sprintf("Short wave radiation day sum: %s %s", first(w.DailyRaw.Radiation), w.DailyUnitsRaw.Radiation), fmt.Sprintf("ShortWave rad: %s %s", pick(w.HourlyRaw.Shortwave), w.HourlyUnitsRaw.Shortwave), fmt.Sprintf("Diffuse rad: %s %s", pick(w.HourlyRaw.Diffuse), w.HourlyUnitsRaw.Diffuse), "\u2009\u2009\u2009 ", "Time: " + rfc(w.Current.Time), "Sun rise: " + rfc(first(w.DailyRaw.Sunrise)), "Sun set: " + rfc(first(w.DailyRaw.Sunset)), "\u2009\u2009\u2009 ", fmt.Sprintf("Soil temp 18cm: %s %s", pick(w.HourlyRaw.SoilTemp), w.HourlyUnitsRaw.SoilTemp), fmt.Sprintf("Soil moist 3-9cm: %s %s", pick(w.HourlyRaw.SoilMoist), w.HourlyUnitsRaw.SoilMoist)}
	return strings.Join(lines, "\\n") + "\\n"
}
func first(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}

type TimeService struct {
	HTTP                            *http.Client
	GeoURL, SunriseURL, TimezoneURL string
}

func (s *TimeService) Get(ctx context.Context, location string) (string, error) {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	geo := s.GeoURL
	if geo == "" {
		geo = "http://api.geonames.org/search"
	}
	u, _ := url.Parse(geo)
	q := u.Query()
	q.Set("q", location)
	q.Set("maxRows", "1")
	q.Set("username", "dev1")
	u.RawQuery = q.Encode()
	var g struct {
		Results []struct {
			Country string `json:"countryName"`
			Lat     string `json:"lat"`
			Lng     string `json:"lng"`
		} `json:"geonames"`
	}
	if e := getJSON(ctx, s.HTTP, u.String(), &g); e != nil {
		return "", e
	}
	if len(g.Results) == 0 {
		return "", fmt.Errorf("location not found")
	}
	r := g.Results[0]
	sun := s.SunriseURL
	if sun == "" {
		sun = "https://api.sunrisesunset.io/json?lat=%s&lng=%s"
	}
	var sr struct {
		Results struct {
			Date      string `json:"date"`
			Sunrise   string `json:"sunrise"`
			Sunset    string `json:"sunset"`
			First     string `json:"first_light"`
			Last      string `json:"last_light"`
			Dawn      string `json:"dawn"`
			Dusk      string `json:"dusk"`
			Noon      string `json:"solar_noon"`
			Golden    string `json:"golden_hour"`
			Length    string `json:"day_length"`
			UTCOffset int    `json:"utc_offset"`
		} `json:"results"`
	}
	if e := getJSON(ctx, s.HTTP, fmt.Sprintf(sun, r.Lat, r.Lng), &sr); e != nil {
		return "", e
	}
	tz := s.TimezoneURL
	if tz == "" {
		tz = "https://timeapi.io/api/Time/current/coordinate?latitude=%s&longitude=%s"
	}
	var tr struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	}
	if e := getJSON(ctx, s.HTTP, fmt.Sprintf(tz, r.Lat, r.Lng), &tr); e != nil {
		return "", e
	}
	current := tr.DateTime
	if parsed, e := time.Parse(time.RFC3339, current); e == nil {
		loc, _ := time.LoadLocation(tr.TimeZone)
		local := parsed
		if loc != nil {
			local = parsed.In(loc)
		}
		current = local.Format(time.RFC1123)
	}
	offset := strconv.Itoa(sr.Results.UTCOffset / 60)
	if sr.Results.UTCOffset > 0 {
		offset = "+" + offset
	}
	payload := fmt.Sprintf("today: %s\\n\\ntime: %s\\n\\nzone: %s\\n\\nUTC offset: %s\\n\\nsun rise: %s\\n\\nsun set: %s\\n\\nfirst light: %s\\n\\nlast light: %s\\n\\ndawn: %s\\n\\ndusk: %s\\n\\nsolar noon: %s\\n\\ngolden hour: %s\\n\\nday length: %s\\n", sr.Results.Date, current, tr.TimeZone, offset, sr.Results.Sunrise, sr.Results.Sunset, sr.Results.First, sr.Results.Last, sr.Results.Dawn, sr.Results.Dusk, sr.Results.Noon, sr.Results.Golden, sr.Results.Length)
	return fmt.Sprintf("\\n Time: **%s, %s** \\n ", location, r.Country) + strings.ReplaceAll(payload, ":", "\u2009:"), nil
}

type SearchService struct {
	HTTP     *http.Client
	Endpoint string
}

func (s *SearchService) Search(ctx context.Context, q string) (string, error) {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	ep := s.Endpoint
	if ep == "" {
		ep = "https://api.duckduckgo.com/"
	}
	u := ep + "?q=" + strings.ReplaceAll(q, " ", "%20") + "&format=json&pretty=1"
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("User-Agent", "Firefox 59.9.0, HC")
	r, e := s.HTTP.Do(req)
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil {
		return "", e
	}
	if r.StatusCode != http.StatusOK {
		return `Please pay for the service requested.`, nil
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(b), `"`, `\\"`), "\n", `\\n`), nil
}

type SCPService struct {
	HTTP     *http.Client
	Endpoint string
	Random   func(int, int) int
}

func (s *SCPService) Description(ctx context.Context, id int) (string, error) {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if id == 0 {
		if s.Random != nil {
			id = s.Random(1, 5500)
		} else {
			id = rand.Intn(5499) + 1
		}
	}
	ep := s.Endpoint
	if ep == "" {
		ep = "https://www.scpwiki.com/scp-%d"
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(ep, id), nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("User-Agent", "Firefox 59.9.0-custom-branch, HC SCP Community")
	r, e := s.HTTP.Do(req)
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != http.StatusOK {
		return "Classified.", nil
	}
	text := string(b)
	a := strings.Index(text, "<strong>Description:</strong>")
	if a >= 0 {
		text = text[a+len("<strong>Description:</strong>"):]
		if b := strings.Index(text, "</p>"); b >= 0 {
			text = text[:b]
		}
	}
	return text, nil
}
func getJSON(ctx context.Context, c *http.Client, u string, v any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return e
	}
	r, e := c.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("http status %d", r.StatusCode)
	}
	return json.NewDecoder(r.Body).Decode(v)
}
