#!/bin/sh
# Runs on first MySQL initialization (empty data volume). Creates the
# application DB AND the isolated test DB in a single boot, and grants the
# application user access to both. This means `docker compose up` and
# `./run_tests.sh` can share one volume without the test run clobbering the
# app DB or vice-versa.

set -eu

APP_DB="${DB_NAME:-service_portal}"
TEST_DB="${DB_TEST_NAME:-service_portal_test}"
APP_USER="${DB_USER:-portal_user}"

mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" <<SQL
CREATE DATABASE IF NOT EXISTS \`${APP_DB}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS \`${TEST_DB}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON \`${APP_DB}\`.* TO '${APP_USER}'@'%';
GRANT ALL PRIVILEGES ON \`${TEST_DB}\`.* TO '${APP_USER}'@'%';
FLUSH PRIVILEGES;
SQL
