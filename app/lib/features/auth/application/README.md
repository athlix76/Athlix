# auth/application

`auth_flow_controller.dart`: phone → OTP state, resend cooldown from config, saves tokens on success.

## Changing it safely

Keep decision logic pure so it can be unit tested in `app/test/`.
