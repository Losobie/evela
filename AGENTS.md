# Build output

For Windows builds, run `go build -ldflags="-H=windowsgui" -o evela.exe .` and replace the existing `evela.exe`. Do not create differently named executable builds.

After each successful Windows build, create `evela.zip` containing only `evela.exe`. Replace any existing `evela.zip` using `Compress-Archive -LiteralPath evela.exe -DestinationPath evela.zip -Force`.

# Git commits

After completing and verifying each requested change, commit the changes using Git with a concise, descriptive commit message. Stage only changes belonging to the current task and preserve unrelated user changes. Do not amend or rewrite existing commits unless explicitly requested.

# Parsed-log cache

When changing extracted log fields, parsing semantics, source classification, relevant embedded catalogs, or the serialized `parsedLog` format, increment `logParserVersion` in `log_cache.go` so unchanged source logs are reprocessed instead of loading stale SQLite results. Database schema changes also require a migration and a `logCacheSchemaVersion` update; parser-version changes alone do not require a schema migration.
