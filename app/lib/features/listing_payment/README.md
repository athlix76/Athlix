# features/listing_payment

Listing fee for publishing an event: quote, order, Razorpay checkout (or mock in non-prod), confirm. This is the only payment Athlix takes; entry fees are never collected.

## Endpoints

- `GET /listing/quote`
- `POST /events/:id/listing/quote`
- `POST /events/:id/listing/order`
- `GET /events/:id/listing/order` (latest order + status; polled after checkout)
- `POST /events/:id/listing/confirm`
- `POST /listing/mock/pay`

## Rules

- All amounts integer paise; percentages basis points.
- Razorpay key id comes from the order response, never from the app.

## Changing it safely

Totals are shown from the quote as given; never recompute fees on the client.
