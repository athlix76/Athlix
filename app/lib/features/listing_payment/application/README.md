# listing_payment/application

`listing_checkout_controller.dart` (order → pay → confirm, polls `GET /events/:id/listing/order` until the order is paid), `razorpay_checkout.dart`.

## Changing it safely

Keep decision logic pure so it can be unit tested in `app/test/`.
