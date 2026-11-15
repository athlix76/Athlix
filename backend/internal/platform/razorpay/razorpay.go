// Package razorpay is a minimal Razorpay client for listing-fee payments: create an order,
// fetch a payment, create a refund, and verify checkout and webhook signatures.
//
// Credentials come only from the `razorpay` secret ({"keyId","keySecret","webhookSecret"},
// docs/security.md). Request and response bodies are never logged, and errors never carry a
// body, a key or the secret: only the HTTP status and Razorpay's error code.
package razorpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"athlix/backend/internal/platform/secrets"
)

// DefaultBaseURL is the Razorpay REST API root.
const DefaultBaseURL = "https://api.razorpay.com/v1"

// maxBody caps how much of a response is read (Razorpay responses are small).
const maxBody = 1 << 20

var (
	// ErrNotConfigured means the razorpay secret is missing or incomplete.
	ErrNotConfigured = errors.New("razorpay: not configured")
	// ErrBadID means an id did not look like a Razorpay id (never sent upstream).
	ErrBadID = errors.New("razorpay: malformed id")
)

// Secret is the shape of the `razorpay` secret.
type Secret struct {
	KeyID         string `json:"keyId"`
	KeySecret     string `json:"keySecret"`
	WebhookSecret string `json:"webhookSecret"`
}

// Valid reports whether all three values are present.
func (s Secret) Valid() bool { return s.KeyID != "" && s.KeySecret != "" && s.WebhookSecret != "" }

// Load reads the secret through the cached getter. name is cfg.SecretName(app.SecretRazorpay).
func Load(ctx context.Context, g secrets.Getter, name string) (Secret, error) {
	var s Secret
	if g == nil {
		return s, ErrNotConfigured
	}
	if err := g.GetJSON(ctx, name, &s); err != nil || !s.Valid() {
		return Secret{}, ErrNotConfigured
	}
	return s, nil
}

// Order is the part of a Razorpay order the backend uses.
type Order struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	Status   string `json:"status"`
}

// Payment is the part of a Razorpay payment the backend uses (no payer details).
type Payment struct {
	ID       string `json:"id"`
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"` // created | authorized | captured | refunded | failed
}

// Refund is the part of a Razorpay refund the backend uses.
type Refund struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"` // pending | processed | failed
}

// Client is the small surface listingpayment depends on. Fake it in tests.
type Client interface {
	// KeyID is the public key id the app needs for Checkout (never the secret).
	KeyID() string
	CreateOrder(ctx context.Context, amountPaise int64, receipt string, notes map[string]string) (Order, error)
	FetchPayment(ctx context.Context, paymentID string) (Payment, error)
	CreateRefund(ctx context.Context, paymentID string, amountPaise int64, receipt string) (Refund, error)
	// VerifyPayment checks the Checkout signature HMAC_SHA256(orderId|paymentId, keySecret).
	VerifyPayment(orderID, paymentID, signature string) bool
	// VerifyWebhook checks X-Razorpay-Signature = HMAC_SHA256(raw body, webhookSecret).
	VerifyWebhook(body []byte, signature string) bool
}

// APIError is a non-2xx answer. It holds no body text beyond Razorpay's error code.
type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("razorpay: http %d", e.Status)
	}
	return fmt.Sprintf("razorpay: http %d %s", e.Status, e.Code)
}

// HTTP is the production Client.
type HTTP struct {
	secret Secret
	base   string
	hc     *http.Client
}

// Option configures HTTP.
type Option func(*HTTP)

// WithBaseURL points the client at another API root (tests).
func WithBaseURL(u string) Option { return func(h *HTTP) { h.base = strings.TrimRight(u, "/") } }

// WithHTTPClient replaces the HTTP client (tests).
func WithHTTPClient(c *http.Client) Option { return func(h *HTTP) { h.hc = c } }

// New builds a client. The secret must be Valid.
func New(s Secret, opts ...Option) *HTTP {
	h := &HTTP{secret: s, base: DefaultBaseURL, hc: &http.Client{Timeout: 10 * time.Second}}
	for _, o := range opts {
		o(h)
	}
	return h
}

// FromSecrets loads the secret and builds the client.
func FromSecrets(ctx context.Context, g secrets.Getter, name string, opts ...Option) (*HTTP, error) {
	s, err := Load(ctx, g, name)
	if err != nil {
		return nil, err
	}
	return New(s, opts...), nil
}

// KeyID implements Client.
func (h *HTTP) KeyID() string { return h.secret.KeyID }

// VerifyPayment implements Client.
func (h *HTTP) VerifyPayment(orderID, paymentID, signature string) bool {
	return VerifyPaymentSignature(orderID, paymentID, signature, h.secret.KeySecret)
}

// VerifyWebhook implements Client.
func (h *HTTP) VerifyWebhook(body []byte, signature string) bool {
	return VerifyWebhookSignature(body, signature, h.secret.WebhookSecret)
}

var (
	paymentIDRe = regexp.MustCompile(`^pay_[A-Za-z0-9]{1,40}$`)
	orderIDRe   = regexp.MustCompile(`^order_[A-Za-z0-9]{1,40}$`)
)

// ValidPaymentID reports whether s looks like a Razorpay payment id.
func ValidPaymentID(s string) bool { return paymentIDRe.MatchString(s) }

// ValidOrderID reports whether s looks like a Razorpay order id.
func ValidOrderID(s string) bool { return orderIDRe.MatchString(s) }

// CreateOrder implements Client (POST /orders, currency INR).
func (h *HTTP) CreateOrder(ctx context.Context, amountPaise int64, receipt string, notes map[string]string) (Order, error) {
	body := map[string]any{"amount": amountPaise, "currency": "INR", "receipt": receipt}
	if len(notes) > 0 {
		body["notes"] = notes
	}
	var o Order
	err := h.do(ctx, http.MethodPost, "/orders", body, &o)
	return o, err
}

// FetchPayment implements Client (GET /payments/:id).
func (h *HTTP) FetchPayment(ctx context.Context, paymentID string) (Payment, error) {
	if !ValidPaymentID(paymentID) {
		return Payment{}, ErrBadID
	}
	var p Payment
	err := h.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &p)
	return p, err
}

// CreateRefund implements Client (POST /payments/:id/refund).
func (h *HTTP) CreateRefund(ctx context.Context, paymentID string, amountPaise int64, receipt string) (Refund, error) {
	if !ValidPaymentID(paymentID) {
		return Refund{}, ErrBadID
	}
	body := map[string]any{"amount": amountPaise}
	if receipt != "" {
		body["receipt"] = receipt
	}
	var r Refund
	err := h.do(ctx, http.MethodPost, "/payments/"+url.PathEscape(paymentID)+"/refund", body, &r)
	return r, err
}

func (h *HTTP) do(ctx context.Context, method, path string, in, out any) error {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.base+path, rd)
	if err != nil {
		return errors.New("razorpay: build request failed")
	}
	req.SetBasicAuth(h.secret.KeyID, h.secret.KeySecret)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.hc.Do(req)
	if err != nil {
		// The transport error may echo the URL; it never holds credentials (basic auth is a header).
		return fmt.Errorf("razorpay: request failed: %w", stripURL(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return errors.New("razorpay: read response failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return &APIError{Status: resp.StatusCode, Code: safeCode(e.Error.Code)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("razorpay: bad response")
	}
	return nil
}

var codeRe = regexp.MustCompile(`^[A-Z_]{1,64}$`)

func safeCode(c string) string {
	if codeRe.MatchString(c) {
		return c
	}
	return ""
}

func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// VerifyPaymentSignature checks the Checkout signature with a constant-time compare.
func VerifyPaymentSignature(orderID, paymentID, signature, keySecret string) bool {
	if keySecret == "" || orderID == "" || paymentID == "" {
		return false
	}
	return hmacEqual([]byte(orderID+"|"+paymentID), signature, keySecret)
}

// VerifyWebhookSignature checks X-Razorpay-Signature over the raw body, constant-time.
func VerifyWebhookSignature(body []byte, signature, webhookSecret string) bool {
	if webhookSecret == "" {
		return false
	}
	return hmacEqual(body, signature, webhookSecret)
}

// Sign returns hex HMAC-SHA256(msg, key). Exported for tests and fakes.
func Sign(msg []byte, key string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(msg)
	return hex.EncodeToString(m.Sum(nil))
}

func hmacEqual(msg []byte, signature, key string) bool {
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(got) != sha256.Size {
		return false
	}
	m := hmac.New(sha256.New, []byte(key))
	m.Write(msg)
	return hmac.Equal(got, m.Sum(nil))
}

// WebhookEvent is the parsed part of a webhook body (ids, amounts and statuses only).
type WebhookEvent struct {
	Event   string
	Payment *Payment
	Order   *Order
	Refund  *Refund
}

// ParseWebhook decodes a webhook body that has already passed VerifyWebhook.
func ParseWebhook(body []byte) (WebhookEvent, error) {
	var raw struct {
		Event   string `json:"event"`
		Payload struct {
			Payment *struct {
				Entity Payment `json:"entity"`
			} `json:"payment"`
			Order *struct {
				Entity Order `json:"entity"`
			} `json:"order"`
			Refund *struct {
				Entity Refund `json:"entity"`
			} `json:"refund"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Event == "" {
		return WebhookEvent{}, errors.New("razorpay: bad webhook body")
	}
	ev := WebhookEvent{Event: raw.Event}
	if p := raw.Payload.Payment; p != nil {
		ev.Payment = &p.Entity
	}
	if o := raw.Payload.Order; o != nil {
		ev.Order = &o.Entity
	}
	if r := raw.Payload.Refund; r != nil {
		ev.Refund = &r.Entity
	}
	return ev, nil
}

// Fake is an in-memory Client for tests in any package.
type Fake struct {
	Key           string
	KeySecret     string
	WebhookSecret string
	Orders        []Order
	Refunds       []Refund
	Payments      map[string]Payment
	Err           error // returned by every API call when set
	n             int
}

// KeyID implements Client.
func (f *Fake) KeyID() string { return f.Key }

// CreateOrder implements Client.
func (f *Fake) CreateOrder(_ context.Context, amount int64, receipt string, _ map[string]string) (Order, error) {
	if f.Err != nil {
		return Order{}, f.Err
	}
	f.n++
	o := Order{ID: fmt.Sprintf("order_fake%d", f.n), Amount: amount, Currency: "INR", Receipt: receipt, Status: "created"}
	f.Orders = append(f.Orders, o)
	return o, nil
}

// FetchPayment implements Client.
func (f *Fake) FetchPayment(_ context.Context, id string) (Payment, error) {
	if f.Err != nil {
		return Payment{}, f.Err
	}
	p, ok := f.Payments[id]
	if !ok {
		return Payment{}, &APIError{Status: 404, Code: "BAD_REQUEST_ERROR"}
	}
	return p, nil
}

// CreateRefund implements Client.
func (f *Fake) CreateRefund(_ context.Context, paymentID string, amount int64, _ string) (Refund, error) {
	if f.Err != nil {
		return Refund{}, f.Err
	}
	f.n++
	r := Refund{ID: fmt.Sprintf("rfnd_fake%d", f.n), PaymentID: paymentID, Amount: amount, Status: "pending"}
	f.Refunds = append(f.Refunds, r)
	return r, nil
}

// VerifyPayment implements Client.
func (f *Fake) VerifyPayment(orderID, paymentID, signature string) bool {
	return VerifyPaymentSignature(orderID, paymentID, signature, f.KeySecret)
}

// VerifyWebhook implements Client.
func (f *Fake) VerifyWebhook(body []byte, signature string) bool {
	return VerifyWebhookSignature(body, signature, f.WebhookSecret)
}
