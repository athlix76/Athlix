# startup/presentation

`splash_screen.dart`, `force_update_screen.dart`, `maintenance_screen.dart`, `suspended_screen.dart`. The router decides when each shows.

## Changing it safely

Screens show server messages via `errorMessage`; never log or print user data.
