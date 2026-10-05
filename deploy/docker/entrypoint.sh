#!/bin/sh
set -eu

echo "entrypoint: waiting for postgres..."
until psql "$DATABASE_URL" -c 'SELECT 1' >/dev/null 2>&1; do
  sleep 1
done

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c \
  "CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY);"

for f in /migrations/*.up.sql; do
  name=$(basename "$f")
  applied=$(psql "$DATABASE_URL" -tAc "SELECT 1 FROM schema_migrations WHERE name = '$name'" | tr -d '[:space:]')
  if [ "$applied" = "1" ]; then
    continue
  fi
  echo "entrypoint: applying $name"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations (name) VALUES ('$name');"
done

echo "entrypoint: starting API"
exec /app/server
