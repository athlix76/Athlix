# features/auth

Phone number + OTP sign-in.

## Endpoints

- `POST /auth/otp/request`
- `POST /auth/otp/verify`
- `POST /auth/refresh`
- `POST /auth/logout`

## Rules

- OTP length, TTL and resend cooldown come from config (`auth.otp.*`).
- Tokens go straight to `TokenStore`; never log the OTP, mobile or tokens.

## Changing it safely

Auth calls are sent with `AuthInterceptor.skipAuth`. Keep it that way.
