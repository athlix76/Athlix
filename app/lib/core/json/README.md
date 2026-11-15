# core/json

`json_utils.dart`: tolerant readers (`asInt`, `asIntOrNull`, `asString`, `asBool`, `asList`, `asMapOrNull`,
`asDateTimeOrNull`, ...), RFC 3339 / date-only formatting, and query cleanup (drops nulls).

## Changing it safely

Readers must never throw on bad input; they return a default. Dates are always converted to UTC.
