#!/bin/sh
# Wipes the public demo back to its seed state. Run as root by
# minigame-demo-reset.timer every day, and by install.sh after each deploy.
#
# Steps: stop the API, rotate the JWT secret, revert and re-apply every
# migration, load seed.sql, start the API (which recreates the admin account).
set -eu

APP_DIR=/opt/minigame-demo
ENV_FILE=/etc/minigame-demo/env
RUN_AS=minigame-demo

systemctl stop minigame-demo.service

# Whatever happens below, bring the API back: a demo with an error in the
# journal is better than a dark one.
trap 'systemctl start minigame-demo.service' EXIT

# User ids restart from 1 after the wipe, so a token issued before it could
# name a different account afterwards. A new secret invalidates them all.
secret=$(openssl rand -hex 32)
sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${secret}/" "$ENV_FILE"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

cd "$APP_DIR"
runuser -u "$RUN_AS" -- "$APP_DIR/bin/migrate" -cmd down
runuser -u "$RUN_AS" -- "$APP_DIR/bin/migrate" -cmd up

# MYSQL_PWD keeps the password out of the process list.
MYSQL_PWD="$DB_PASSWORD" mysql --protocol=TCP -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" \
    --default-character-set=utf8mb4 "$DB_NAME" < "$APP_DIR/seed.sql"

echo "minigame-demo: reset complete"
