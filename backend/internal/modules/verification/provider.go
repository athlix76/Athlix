package verification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"athlix/backend/internal/app"
)

// provider is one identity provider. Start returns the URL the app opens and an optional
// provider reference; Complete exchanges the callback code for the identity fields.
type provider interface {
	Start(ctx context.Context, state, challenge string) (authURL, ref string, err error)
	Complete(ctx context.Context, code, verifier, ref string) (Identity, error)
}

// errProvider is returned for any upstream failure. It never carries the upstream body.
var errProvider = errors.New("verification: provider error")

const maxBody = 4 << 20 // eAadhaar XML with photo is ~100 KB; cap reads at 4 MB

// DigiLockerSecret is the "digilocker" secret (docs/security.md#secrets).
type DigiLockerSecret struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RedirectURI  string `json:"redirectUri"`
	AuthorizeURL string `json:"authorizeUrl"`
	TokenURL     string `json:"tokenUrl"`
	APIBaseURL   string `json:"apiBaseUrl"`
}

// AggregatorSecret is the "kyc-aggregator" secret (docs/security.md#secrets).
type AggregatorSecret struct {
	Provider      string `json:"provider"`
	BaseURL       string `json:"baseUrl"`
	APIKey        string `json:"apiKey"`
	APISecret     string `json:"apiSecret"`
	WebhookSecret string `json:"webhookSecret"`
}

// pkceChallenge is the S256 code challenge for verifier.
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// ---- DigiLocker direct (OAuth2 authorization code + PKCE) ----

type direct struct {
	d      *app.Deps
	client *http.Client
}

func (p *direct) secret(ctx context.Context) (DigiLockerSecret, error) {
	var s DigiLockerSecret
	if err := p.d.Secrets.GetJSON(ctx, p.d.Cfg.SecretName(app.SecretDigiLocker), &s); err != nil {
		return s, errProvider
	}
	if s.ClientID == "" || s.AuthorizeURL == "" || s.TokenURL == "" || s.APIBaseURL == "" || s.RedirectURI == "" {
		return s, errProvider
	}
	return s, nil
}

func (p *direct) Start(ctx context.Context, state, challenge string) (string, string, error) {
	s, err := p.secret(ctx)
	if err != nil {
		return "", "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.ClientID)
	q.Set("redirect_uri", s.RedirectURI)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	sep := "?"
	if strings.Contains(s.AuthorizeURL, "?") {
		sep = "&"
	}
	return s.AuthorizeURL + sep + q.Encode(), "", nil
}

type dlToken struct {
	AccessToken string `json:"access_token"`
	DigiLocker  string `json:"digilockerid"`
	Name        string `json:"name"`
	DOB         string `json:"dob"` // DDMMYYYY
	Gender      string `json:"gender"`
}

func (p *direct) Complete(ctx context.Context, code, verifier, _ string) (Identity, error) {
	s, err := p.secret(ctx)
	if err != nil {
		return Identity{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", s.ClientID)
	form.Set("client_secret", s.ClientSecret)
	form.Set("redirect_uri", s.RedirectURI)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, errProvider
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tok dlToken
	if err := doJSON(p.client, req, &tok); err != nil || tok.AccessToken == "" || tok.DigiLocker == "" {
		return Identity{}, errProvider
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.APIBaseURL, "/")+"/3/xml/eaadhaar", nil)
	if err != nil {
		return Identity{}, errProvider
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	tok.AccessToken = "" // the access token is never kept
	resp, err := p.client.Do(req)
	if err != nil {
		return Identity{}, errProvider
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, errProvider
	}
	id, err := parseEAadhaar(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Identity{}, errProvider
	}
	id.SubjectID = tok.DigiLocker
	// The token response carries the same identity fields; fill gaps from it.
	if id.Name == "" {
		id.Name = tok.Name
	}
	if id.DOB.IsZero() {
		id.DOB, _ = parseDOB(tok.DOB)
	}
	if id.Gender == "" {
		id.Gender = normGender(tok.Gender)
	}
	if id.DOB.IsZero() || id.Name == "" {
		return Identity{}, errProvider
	}
	return id, nil
}

// parseEAadhaar streams the eAadhaar XML and reads only the attributes Athlix needs:
// UidData@uid (masked, last 4 kept), Poi@name/dob/gender and Poa@state/dist. Everything else
// (photo, full address, signature) is skipped without being retained.
func parseEAadhaar(r io.Reader) (Identity, error) {
	dec := xml.NewDecoder(r)
	var id Identity
	seen := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Identity{}, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "UidData":
			seen = true
			if uid := attr(se, "uid"); len(uid) >= 4 {
				id.AadhaarLast4 = uid[len(uid)-4:]
			}
		case "Poi":
			id.Name = attr(se, "name")
			id.DOB, _ = parseDOB(attr(se, "dob"))
			id.Gender = normGender(attr(se, "gender"))
		case "Poa":
			id.StateName = attr(se, "state")
			id.DistrictName = attr(se, "dist")
		case "Pht":
			_ = dec.Skip()
		}
	}
	if !seen {
		return Identity{}, errProvider
	}
	if !isDigits(id.AadhaarLast4) {
		id.AadhaarLast4 = ""
	}
	return id, nil
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

// ---- KYC aggregator (hosted DigiLocker session) ----
//
// Wire format (generic hosted-session API; adapt here if the chosen vendor differs):
//
//	POST {baseUrl}/v1/digilocker/sessions  {reference, redirectUrl} -> {sessionId, authorizationUrl}
//	GET  {baseUrl}/v1/digilocker/sessions/{sessionId}/identity      -> aggregatorIdentity
//
// Both calls send X-Api-Key / X-Api-Secret headers. The callback reaches the same
// /verification/digilocker/callback route; the session id comes from Redis, never from the URL.

type aggregator struct {
	d      *app.Deps
	client *http.Client
}

type aggregatorIdentity struct {
	Status        string `json:"status"` // completed | cancelled | failed
	SubjectID     string `json:"subjectId"`
	MaskedAadhaar string `json:"maskedAadhaar"`
	Name          string `json:"name"`
	DOB           string `json:"dob"` // YYYY-MM-DD or DD-MM-YYYY
	Gender        string `json:"gender"`
	Address       struct {
		State    string `json:"state"`
		District string `json:"district"`
	} `json:"address"`
}

// errCancelled is returned when the user cancelled at the provider.
var errCancelled = errors.New("verification: cancelled")

func (p *aggregator) secret(ctx context.Context) (AggregatorSecret, error) {
	var s AggregatorSecret
	if err := p.d.Secrets.GetJSON(ctx, p.d.Cfg.SecretName(app.SecretKYCAggregator), &s); err != nil {
		return s, errProvider
	}
	if s.BaseURL == "" || s.APIKey == "" {
		return s, errProvider
	}
	return s, nil
}

func (p *aggregator) request(ctx context.Context, s AggregatorSecret, method, path string, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.BaseURL, "/")+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", s.APIKey)
	req.Header.Set("X-Api-Secret", s.APISecret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (p *aggregator) Start(ctx context.Context, state, _ string) (string, string, error) {
	s, err := p.secret(ctx)
	if err != nil {
		return "", "", err
	}
	redirect := strings.TrimRight(p.d.Cfg.PublicAPIBaseURL, "/") + "/v1/verification/digilocker/callback?state=" + url.QueryEscape(state)
	req, err := p.request(ctx, s, http.MethodPost, "/v1/digilocker/sessions", map[string]string{"reference": state, "redirectUrl": redirect})
	if err != nil {
		return "", "", errProvider
	}
	var out struct {
		SessionID        string `json:"sessionId"`
		AuthorizationURL string `json:"authorizationUrl"`
	}
	if err := doJSON(p.client, req, &out); err != nil || out.SessionID == "" || out.AuthorizationURL == "" {
		return "", "", errProvider
	}
	if len(out.SessionID) > 128 {
		return "", "", errProvider
	}
	return out.AuthorizationURL, out.SessionID, nil
}

func (p *aggregator) Complete(ctx context.Context, _, _, ref string) (Identity, error) {
	s, err := p.secret(ctx)
	if err != nil {
		return Identity{}, err
	}
	req, err := p.request(ctx, s, http.MethodGet, "/v1/digilocker/sessions/"+url.PathEscape(ref)+"/identity", nil)
	if err != nil {
		return Identity{}, errProvider
	}
	var a aggregatorIdentity
	if err := doJSON(p.client, req, &a); err != nil {
		return Identity{}, errProvider
	}
	switch a.Status {
	case "completed":
	case "cancelled":
		return Identity{}, errCancelled
	default:
		return Identity{}, errProvider
	}
	id := Identity{
		SubjectID: a.SubjectID, Name: strings.TrimSpace(a.Name), Gender: normGender(a.Gender),
		StateName: a.Address.State, DistrictName: a.Address.District,
	}
	if m := strings.TrimSpace(a.MaskedAadhaar); len(m) >= 4 && isDigits(m[len(m)-4:]) {
		id.AadhaarLast4 = m[len(m)-4:]
	}
	id.DOB, _ = parseDOB(a.DOB)
	if id.SubjectID == "" || id.Name == "" || id.DOB.IsZero() {
		return Identity{}, errProvider
	}
	return id, nil
}

// ---- helpers ----

// doJSON sends req and decodes a 2xx JSON body into out. Upstream bodies are never logged.
func doJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return errProvider
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return fmt.Errorf("%w: status %d", errProvider, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return errProvider
	}
	return nil
}

// parseDOB accepts DDMMYYYY, DD-MM-YYYY, DD/MM/YYYY and YYYY-MM-DD.
func parseDOB(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"02012006", "02-01-2006", "02/01/2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errProvider
}

func normGender(g string) string {
	switch strings.ToUpper(strings.TrimSpace(g)) {
	case "M", "MALE":
		return "M"
	case "F", "FEMALE":
		return "F"
	case "T", "TRANSGENDER", "O", "OTHER":
		return "T"
	default:
		return "U"
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
