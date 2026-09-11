package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
	"zenbot/internal/common"
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
	if s == nil {
		return "", fmt.Errorf("last-online persistence unavailable")
	}
	var record repository.LastOnlineRecord
	var err error
	switch {
	case common.DependencyConfigured(s.Queries):
		record, err = s.Queries.LastOnline(ctx, target)
	case common.DependencyConfigured(s.LastSeen):
		record, err = s.LastSeen.LastSeen(ctx, target)
	default:
		return "", fmt.Errorf("last-online persistence unavailable")
	}
	if err != nil {
		return "", err
	}
	if !record.Found {
		return "", repository.ErrNotFound
	}
	lastObserved, presence, message := " - ", " - ", " - "
	observedMillis := record.LastMessageMillis
	if record.LastPresenceMillis.Valid && (!observedMillis.Valid || record.LastPresenceMillis.Int64 > observedMillis.Int64) {
		observedMillis = record.LastPresenceMillis
	}
	if observedMillis.Valid {
		lastObserved, err = util.FormatRFC1123(observedMillis.Int64, util.UnitMilliseconds, "UTC")
		if err != nil {
			return "", err
		}
	}
	if record.LastPresenceMillis.Valid && record.LastPresenceEvent.Valid {
		stamp, err := util.FormatRFC1123(record.LastPresenceMillis.Int64, util.UnitMilliseconds, "UTC")
		if err != nil {
			return "", err
		}
		presence = record.LastPresenceEvent.String + " at " + stamp
	}
	if record.LastMessageMillis.Valid && record.LastMessage.Valid {
		stamp, err := util.FormatRFC1123(record.LastMessageMillis.Int64, util.UnitMilliseconds, "UTC")
		if err != nil {
			return "", err
		}
		message = stamp + " — " + escapeJSON(record.LastMessage.String)
	}
	return fmt.Sprintf("\\n Nick|Trip: %s\\n Last observed: %s\\n Last presence event: %s\\n Last public message: %s\\n", target, lastObserved, presence, message), nil
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
func (s *UserService) IsNameRegistered(ctx context.Context, name string) (bool, error) {
	return s.Identity.IsNameRegistered(ctx, name)
}
func (s *UserService) IsTripRegistered(ctx context.Context, trip string) (bool, error) {
	return s.Identity.IsTripRegistered(ctx, trip)
}
func (s *UserService) Register(ctx context.Context, name, trip string, role model.Role) error {
	err := s.Identity.Register(ctx, name, trip, role)
	if err == nil {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
func (s *UserService) RegisterNameByTrip(ctx context.Context, name, trip string) error {
	err := s.Identity.RegisterNameByTrip(ctx, name, trip)
	if err == nil {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
func (s *UserService) RegisterTripByName(ctx context.Context, name, trip string) error {
	err := s.Identity.RegisterTripByName(ctx, name, trip)
	if err == nil {
		common.RecordCommittedMutation(ctx)
	}
	return err
}
func (s *UserService) LastMessages(ctx context.Context, name, trip string, count int) ([]model.Message, error) {
	return s.Identity.LastMessages(ctx, name, trip, count)
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

func (s *UserService) DeleteIdentity(ctx context.Context, nameOrTrip string) (repository.DeleteResult, error) {
	capability, ok := s.GroupB.(repository.SaturnAuthorizedDeleteRepository)
	if !ok {
		return repository.DeleteResult{}, fmt.Errorf("authorized Group B delete unavailable")
	}
	result, err := capability.DeleteIdentityAuthorized(ctx, nameOrTrip)
	if err == nil && result.TripNamesRows+result.TripRows+result.NameRows > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return result, err
}

func (s *UserService) SaturnRegisteredUsers(ctx context.Context) ([]repository.SaturnRegisteredUser, error) {
	if s == nil || !common.DependencyConfigured(s.GroupB) {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnRegisteredUsers(ctx)
}

// SaturnLastMessages exposes the Saturn-shaped compatibility read without
// changing the existing Zenbot history contract.
func (s *UserService) SaturnLastMessages(ctx context.Context, name *string, trip string, count int) ([]repository.SaturnLastMessage, error) {
	if s == nil || !common.DependencyConfigured(s.GroupB) {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnLastMessages(ctx, name, trip, count)
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
	// A one-day forecast is far smaller than 1 MiB. Bound streaming bodies too,
	// and reject trailing data without decoding an arbitrary second JSON value.
	const maxProviderJSON = 1 << 20
	body, err := io.ReadAll(io.LimitReader(r.Body, maxProviderJSON+1))
	if err != nil {
		return err
	}
	if len(body) > maxProviderJSON {
		return fmt.Errorf("provider JSON exceeds %d bytes", maxProviderJSON)
	}
	return json.Unmarshal(body, v)
}
