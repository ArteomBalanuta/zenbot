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
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/util"
)

// CommandOutput is the only output seam used by migrated command services.
type CommandOutput interface {
	Chat(author, text string, whisper bool) error
	Raw(payload any) error
}

// Bundle contains the non-agent services attached to an engine.
type Bundle struct {
	Security   *SecurityService
	Mail       *MailService
	Notes      *NoteService
	Users      *UserService
	Ping       *PingService
	Weather    *WeatherService
	Time       *TimeService
	Search     *SearchService
	YouTube    *YouTubeService
	SCP        *SCPService
	DBZ        *DBZService
	Activity   *ActivityService
	ShadowBans *ShadowBanService
	SQLCommand RawSQLQuery
}

type UserService struct {
	Queries  repository.UserQueryRepository
	Identity repository.IdentityRepository
	LastSeen repository.LastSeenRepository
	GroupB   repository.SqlUtilGroupBRepository
	Now      func() time.Time
}

func (s *UserService) LastOnline(ctx context.Context, target string) (string, error) {
	if s.Queries == nil {
		return s.lastOnlineFromLastSeen(ctx, target)
	}
	record, err := s.Queries.LastOnline(ctx, target)
	if err != nil {
		return "", err
	}
	if !record.Found {
		return "", repository.ErrNotFound
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	joined, lastSeen, seenActive, sessionDuration, lastMessage := " - ", " - ", " - ", " - ", " - "
	if record.LastSeenMillis.Valid {
		lastSeen, err = util.FormatRFC1123(record.LastSeenMillis.Int64, util.UnitMilliseconds, "UTC")
		if err != nil {
			return "", err
		}
		seenActive = util.Difference(now, time.UnixMilli(record.LastSeenMillis.Int64).UTC())
	}
	if record.LastMessage.Valid {
		lastMessage = escapeJSON(record.LastMessage.String)
	}
	// Saturn only renders session data when its last-seen lookup returned a row.
	if record.LastSeenMillis.Valid && record.JoinedMillis.Valid {
		joined, err = util.FormatRFC1123(record.JoinedMillis.Int64, util.UnitMilliseconds, "UTC")
		if err != nil {
			return "", err
		}
		sessionDuration = util.Difference(now, time.UnixMilli(record.JoinedMillis.Int64).UTC())
	}
	return fmt.Sprintf("\\n Nick|Trip: %s\\n Joined: %s\\n Last seen: %s\\n Seen active: %s ago.\\n Session duration: %s \\n Last message: %s\\n", target, joined, lastSeen, seenActive, sessionDuration, lastMessage), nil
}

func escapeJSON(value string) string {
	var escaped strings.Builder
	for _, character := range value {
		switch character {
		case '\\':
			escaped.WriteString("\\\\")
		case '"':
			escaped.WriteString("\\\"")
		case '\b':
			escaped.WriteString("\\b")
		case '\f':
			escaped.WriteString("\\f")
		case '\n':
			escaped.WriteString("\\n")
		case '\r':
			escaped.WriteString("\\r")
		case '	':
			escaped.WriteString(`	`)
		default:
			if character < 0x20 {
				fmt.Fprintf(&escaped, "\\u%04x", character)
			} else {
				escaped.WriteRune(character)
			}
		}
	}
	return escaped.String()
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

func (s *UserService) SeenRecently(ctx context.Context, user *model.User) (string, error) {
	if s == nil || user == nil || s.Queries == nil {
		return "", nil
	}
	recent, ok := s.Queries.(repository.RecentPresenceRepository)
	if !ok {
		return "", nil
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	names, err := recent.RecentPresenceNames(ctx, user.Hash, user.Trip, now.Add(-15*time.Minute).UnixMilli(), 5)
	if err != nil {
		return "", err
	}
	aliases := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" || strings.EqualFold(name, user.Name) {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, name)
	}
	if len(aliases) == 0 {
		return "", nil
	}
	return fmt.Sprintf("\\n @%s, has been seen as: _%s_ recently. \\n", user.Name, strings.Join(aliases, ", ")), nil
}

func (s *UserService) lastOnlineFromLastSeen(ctx context.Context, target string) (string, error) {
	if s.LastSeen == nil {
		return "", fmt.Errorf("last-online persistence unavailable")
	}
	record, err := s.LastSeen.LastSeen(ctx, target)
	if err != nil {
		return "", err
	}
	format := func(timestamp *int64) string {
		if timestamp == nil {
			return " - "
		}
		return time.UnixMilli(*timestamp).In(time.FixedZone("GMT", 0)).Format(time.RFC1123)
	}
	message := record.Message
	if message == "" {
		message = " - "
	}
	message = strings.ReplaceAll(message, `\`, `\\`)
	message = strings.ReplaceAll(message, `"`, `\"`)
	return fmt.Sprintf("\\n Nick|Trip: %s\\n Joined: %s\\n Last seen: %s\\n Seen active: -  ago.\\n Session duration: -  \\n Last message: %s\\n", target, format(record.JoinedAt), format(record.SeenAt), message), nil
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
	// An exact registered trip takes precedence over a nickname with the same
	// spelling. Trip identities are case-sensitive; only nickname lookup folds case.
	rows, e := s.DB.Query(`SELECT DISTINCT t.trip FROM trips t
		WHERE t.trip=$2 OR (NOT EXISTS (SELECT 1 FROM trips WHERE trip=$2) AND EXISTS (
			SELECT 1 FROM trip_names tn INNER JOIN names n ON tn.name_id=n.id
			WHERE tn.trip_id=t.id AND LOWER(n.name)=$1
		)) ORDER BY t.trip`, strings.ToLower(receiver), receiver)
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
	if _, err := s.DB.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES($1,$2,$3,'PENDING',$4,$5)`, owner, receivers, message, time.Now().UnixMilli(), strconv.FormatBool(whisper)); err != nil {
		return "", err
	}
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
	// Receivers are resolved trips, never claimant-controlled nicknames. Keep
	// the nickname argument for callers, but authenticate delivery only by trip.
	if strings.TrimSpace(trip) == "" || strings.Contains(trip, ",") {
		return nil, nil
	}
	rows, e := s.DB.Query(`SELECT id,owner,receiver,message,status,created_on,is_whisper FROM mail WHERE status='PENDING' AND LOCATE(',' || $1 || ',', ',' || receiver || ',') > 0 ORDER BY id`, trip)
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
	lines := []string{fmt.Sprintf("Weather forecast for today: **%s**", area), fmt.Sprintf("Temperature: %s %s", w.Current.Temperature, w.CurrentUnits.Temperature), fmt.Sprintf("Feels temp: %s %s", pick(w.HourlyRaw.Apparent), w.HourlyUnitsRaw.Apparent), fmt.Sprintf("Air Humidity: %s %s", pick(w.HourlyRaw.Humidity), w.HourlyUnitsRaw.Humidity), "Precipitation: " + code, fmt.Sprintf("Wind speed: %s %s", w.Current.Windspeed, w.CurrentUnits.Windspeed), fmt.Sprintf("Pressure surface: %s %s", pick(w.HourlyRaw.Surface), w.HourlyUnitsRaw.Surface), fmt.Sprintf("Pressure sea level: %s %s", pick(w.HourlyRaw.Sea), w.HourlyUnitsRaw.Sea), fmt.Sprintf("UV day max index: %s %s", first(w.DailyRaw.UV), w.DailyUnitsRaw.UV), fmt.Sprintf("Short wave radiation day sum: %s %s", first(w.DailyRaw.Radiation), w.DailyUnitsRaw.Radiation), fmt.Sprintf("ShortWave rad: %s %s", pick(w.HourlyRaw.Shortwave), w.HourlyUnitsRaw.Shortwave), fmt.Sprintf("Diffuse rad: %s %s", pick(w.HourlyRaw.Diffuse), w.HourlyUnitsRaw.Diffuse), "Time: " + rfc(w.Current.Time), "Sun rise: " + rfc(first(w.DailyRaw.Sunrise)), "Sun set: " + rfc(first(w.DailyRaw.Sunset)), fmt.Sprintf("Soil temp 18cm: %s %s", pick(w.HourlyRaw.SoilTemp), w.HourlyUnitsRaw.SoilTemp), fmt.Sprintf("Soil moist 3-9cm: %s %s", pick(w.HourlyRaw.SoilMoist), w.HourlyUnitsRaw.SoilMoist)}
	return alignLiteralLines(lines, true)
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
	lines := []string{
		"today: " + sr.Results.Date,
		"time: " + current,
		"zone: " + tr.TimeZone,
		"UTC offset: " + offset,
		"sun rise: " + sr.Results.Sunrise,
		"sun set: " + sr.Results.Sunset,
		"first light: " + sr.Results.First,
		"last light: " + sr.Results.Last,
		"dawn: " + sr.Results.Dawn,
		"dusk: " + sr.Results.Dusk,
		"solar noon: " + sr.Results.Noon,
		"golden hour: " + sr.Results.Golden,
		"day length: " + sr.Results.Length,
	}
	return fmt.Sprintf("\\n Time: **%s, %s** \\n ", location, r.Country) + alignLiteralLines(lines, false), nil
}

func alignLiteralLines(lines []string, prepend bool) string {
	maxWidth := 0
	for _, line := range lines {
		key, _, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if width := utf8.RuneCountInString(key); width > maxWidth {
			maxWidth = width
		}
	}
	var output strings.Builder
	for _, line := range lines {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		padding := strings.Repeat("\u2009", maxWidth-utf8.RuneCountInString(key))
		if prepend {
			output.WriteString(padding)
			output.WriteString(key)
		} else {
			output.WriteString(key)
			output.WriteString(padding)
		}
		output.WriteByte(':')
		output.WriteString(value)
		output.WriteString(`\n`)
	}
	return output.String()
}

type SearchService struct {
	HTTP     *http.Client
	Endpoint string
}

type YouTubeService struct {
	HTTP     *http.Client
	Endpoint string
}

func (s *YouTubeService) Preview(ctx context.Context, message string) (string, bool, error) {
	videoID := extractYouTubeID(message)
	if videoID == "" {
		return "", false, nil
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = "https://www.youtube.com/oembed"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", true, err
	}
	query := u.Query()
	query.Set("format", "json")
	query.Set("url", "https://youtube.com/watch?v="+videoID)
	u.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", true, err
	}
	request.Header.Set("User-Agent", "Firefox 59.9.0")
	response, err := client.Do(request)
	if err != nil {
		return "", true, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", true, fmt.Errorf("YouTube metadata status %d", response.StatusCode)
	}
	var metadata struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		return "", true, err
	}
	if strings.TrimSpace(metadata.Title) == "" {
		return "", true, fmt.Errorf("YouTube metadata title is blank")
	}
	preview := fmt.Sprintf("Title: %s\\n![%s](https://i.ytimg.com/vi/%s/hqdefault.jpg)", metadata.Title, metadata.Title, videoID)
	return preview, true, nil
}

func extractYouTubeID(message string) string {
	for _, field := range strings.Fields(message) {
		candidate := strings.Trim(field, "<>()[]{}\"',.!;")
		if !strings.Contains(candidate, "://") {
			continue
		}
		u, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
		switch host {
		case "youtube.com", "m.youtube.com":
			if id := strings.TrimSpace(u.Query().Get("v")); id != "" {
				return id
			}
		case "youtu.be":
			if id := strings.TrimSpace(path.Base(u.Path)); id != "" && id != "." && id != "/" {
				return id
			}
		}
	}
	return ""
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
