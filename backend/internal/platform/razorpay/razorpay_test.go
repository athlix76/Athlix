package razorpay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"athlix/backend/internal/platform/secrets"
)

var testSecret = Secret{KeyID: "rzp_test_key", KeySecret: "ks_test", WebhookSecret: "wh_test"}

func TestVerifyPaymentSignature(t *testing.T) {
	sig := Sign([]byte("order_A|pay_B"), "ks_test")
	if !VerifyPaymentSignature("order_A", "pay_B", sig, "ks_test") {
		t.Fatal("valid signature rejected")
	}
	for _, c := range []struct{ o, p, s, k string }{
		{"order_A", "pay_C", sig, "ks_test"},
		{"order_A", "pay_B", sig, "other"},
		{"order_A", "pay_B", "zz", "ks_test"},
		{"order_A", "pay_B", sig[:10], "ks_test"},
		{"order_A", "pay_B", sig, ""},
		{"", "pay_B", sig, "ks_test"},
	} {
		if VerifyPaymentSignature(c.o, c.p, c.s, c.k) {
			t.Fatalf("accepted bad signature %+v", c)
		}
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"event":"payment.captured"}`)
	sig := Sign(body, "wh_test")
	if !VerifyWebhookSignature(body, sig, "wh_test") {
		t.Fatal("valid webhook rejected")
	}
	if VerifyWebhookSignature(append(body, ' '), sig, "wh_test") {
		t.Fatal("tampered body accepted")
	}
	if VerifyWebhookSignature(body, sig, "") {
		t.Fatal("empty secret accepted")
	}
	c := New(testSecret)
	if !c.VerifyWebhook(body, sig) || c.VerifyWebhook(body, Sign(body, "ks_test")) {
		t.Fatal("client must use the webhook secret, not the key secret")
	}
	if !c.VerifyPayment("order_A", "pay_B", Sign([]byte("order_A|pay_B"), "ks_test")) {
		t.Fatal("client must use the key secret for checkout signatures")
	}
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	g := secrets.Static{"athlix/test/razorpay": testSecret}
	s, err := Load(ctx, g, "athlix/test/razorpay")
	if err != nil || s.KeyID != "rzp_test_key" {
		t.Fatalf("load: %v %+v", err, s)
	}
	if _, err := Load(ctx, secrets.Static{}, "athlix/test/razorpay"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing secret: %v", err)
	}
	half := secrets.Static{"x": map[string]string{"keyId": "k"}}
	if _, err := Load(ctx, half, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("incomplete secret: %v", err)
	}
	if _, err := FromSecrets(ctx, g, "athlix/test/razorpay"); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPClient(t *testing.T) {
	var gotAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		gotAuth = ok && u == "rzp_test_key" && p == "ks_test"
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/orders":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if in["currency"] != "INR" || in["amount"].(float64) != 47200 {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"id":"order_X1","amount":47200,"currency":"INR","receipt":"lo_1","status":"created"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/payments/pay_P1":
			_, _ = w.Write([]byte(`{"id":"pay_P1","order_id":"order_X1","amount":47200,"status":"captured","email":"x@y"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/payments/pay_P1/refund":
			_, _ = w.Write([]byte(`{"id":"rfnd_R1","payment_id":"pay_P1","amount":1000,"status":"pending"}`))
		default:
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"secret ks_test leaked?"}}`))
		}
	}))
	defer srv.Close()
	c := New(testSecret, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	ctx := context.Background()

	o, err := c.CreateOrder(ctx, 47200, "lo_1", map[string]string{"eventId": "9"})
	if err != nil || o.ID != "order_X1" || !gotAuth {
		t.Fatalf("create order: %v %+v auth=%v", err, o, gotAuth)
	}
	p, err := c.FetchPayment(ctx, "pay_P1")
	if err != nil || p.Status != "captured" || p.OrderID != "order_X1" {
		t.Fatalf("fetch payment: %v %+v", err, p)
	}
	r, err := c.CreateRefund(ctx, "pay_P1", 1000, "lo_1")
	if err != nil || r.ID != "rfnd_R1" {
		t.Fatalf("refund: %v %+v", err, r)
	}
	_, err = c.FetchPayment(ctx, "pay_Missing")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 400 || ae.Code != "BAD_REQUEST_ERROR" {
		t.Fatalf("api error: %v", err)
	}
	if strings.Contains(err.Error(), "ks_test") || strings.Contains(err.Error(), "leaked") {
		t.Fatal("error must not carry the response body")
	}
	if _, err := c.FetchPayment(ctx, "../orders"); !errors.Is(err, ErrBadID) {
		t.Fatalf("path injection: %v", err)
	}
	if _, err := c.CreateRefund(ctx, "pay_1/x", 1, ""); !errors.Is(err, ErrBadID) {
		t.Fatalf("path injection: %v", err)
	}
}

func TestParseWebhook(t *testing.T) {
	body := []byte(`{"entity":"event","event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_1","order_id":"order_1","amount":100,"status":"captured","contact":"+91..."}}}}`)
	ev, err := ParseWebhook(body)
	if err != nil || ev.Event != "payment.captured" || ev.Payment == nil || ev.Payment.OrderID != "order_1" || ev.Order != nil {
		t.Fatalf("parse: %v %+v", err, ev)
	}
	ev, err = ParseWebhook([]byte(`{"event":"refund.processed","payload":{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_1","amount":50,"status":"processed"}}}}`))
	if err != nil || ev.Refund == nil || ev.Refund.Amount != 50 {
		t.Fatalf("parse refund: %v %+v", err, ev)
	}
	if _, err := ParseWebhook([]byte(`nope`)); err == nil {
		t.Fatal("bad body accepted")
	}
}

func TestIDValidators(t *testing.T) {
	if !ValidPaymentID("pay_ABC123") || ValidPaymentID("pay_") || ValidPaymentID("order_1") {
		t.Fatal("payment id validator")
	}
	if !ValidOrderID("order_ABC") || ValidOrderID("order_A B") {
		t.Fatal("order id validator")
	}
}

func TestFake(t *testing.T) {
	f := &Fake{Key: "k", KeySecret: "s", WebhookSecret: "w"}
	var _ Client = f
	var _ Client = New(testSecret)
	o, _ := f.CreateOrder(context.Background(), 10, "r", nil)
	if o.ID == "" || len(f.Orders) != 1 {
		t.Fatal("fake order")
	}
	f.Err = errors.New("down")
	if _, err := f.CreateRefund(context.Background(), "pay_1", 1, ""); err == nil {
		t.Fatal("fake error")
	}
}
