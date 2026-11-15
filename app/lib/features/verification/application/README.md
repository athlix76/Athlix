# verification/application

`verification_controller.dart`: opens the DigiLocker URL, polls status after the deep link.

## Changing it safely

Keep decision logic pure so it can be unit tested in `app/test/`.
