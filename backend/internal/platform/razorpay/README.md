# platform/razorpay

Minimal Razorpay client for the listing fee (module `listingpayment`): orders, payment fetch,
refunds, and Checkout / webhook signature checks. Entry fees never go through Razorpay.

## API

| Name | Use |
|---|---|
| `Client` | Interface listingpayment depends on: `KeyID`, `CreateOrder`, `FetchPayment`, `CreateRefund`, `VerifyPayment`, `VerifyWebhook` |
| `Load(ctx, getter, name)` / `FromSecrets(ctx, getter, name, ...Option)` | Read the `razorpay` secret (`cfg.SecretName(app.SecretRazorpay)`) and build the `HTTP` client. `ErrNotConfigured` when missing or incomplete |
| `New(Secret, ...Option)` | Production client. Options: `WithBaseURL`, `WithHTTPClient` (tests) |
| `VerifyPaymentSignature(orderID, paymentID, sig, keySecret)` | Checkout signature: hex `HMAC_SHA256(orderId + "|" + paymentId, keySecret)` |
| `VerifyWebhookSignature(body, sig, webhookSecret)` | `X-Razorpay-Signature`: hex `HMAC_SHA256(raw body, webhookSecret)` |
| `ParseWebhook(body)` | Ids, amounts and statuses of `payment`, `order`, `refund` entities. Call only after the signature passed |
| `ValidPaymentID`, `ValidOrderID` | Shape checks; ids failing them are never sent upstream (`ErrBadID`) |
| `Sign(msg, key)` | Hex HMAC for tests and fakes |
| `Fake` | In-memory `Client` for tests in any package |
| `APIError{Status, Code}` | Non-2xx answer; carries Razorpay's error code only |

## Rules

- Credentials come only from the `razorpay` secret `{"keyId","keySecret","webhookSecret"}`
  (docs/security.md). `KeyID()` is the public key id the app needs for Checkout; the key
  secret and webhook secret never leave this package.
- Signatures are compared in constant time (`hmac.Equal`) over the **raw** body.
- Request and response bodies are never logged or put into errors (payer contact details
  live in them). Errors carry the HTTP status and Razorpay's error code only.
- Payment ids from clients are validated before they reach a URL path.
- HTTP timeout 10 s; responses are read up to 1 MiB.

## Tests

`razorpay_test.go`: signature checks (valid, tampered, wrong key, empty secret, truncated),
secret loading, the HTTP client against a local server (basic auth, paths, error mapping
without the body, path-injection refusal), webhook parsing and the fake.
