set dotenv-load := true

SUDO_POSTGRES := if `uname` == "Linux" { "sudo -u postgres" } else { "" }

# create a user and database in PostgreSQL
initdb:
    {{SUDO_POSTGRES}} createuser --no-createdb --no-createrole --no-superuser --login --inherit ${COA_DB_USER}
    {{SUDO_POSTGRES}} psql -c "alter user ${COA_DB_USER} with encrypted password '${COA_DB_PASSWORD}';"
    {{SUDO_POSTGRES}} createdb --encoding=utf-8 --owner=${COA_DB_USER} ${COA_DB_NAME}

# create migration up and down files in the ./migrations directory
create-migration name:
    migrate create -ext sql -dir migrations {{name}}

# apply migrations from the ./migrations directory
migrate:
    migrate -path migrations -database postgres://${COA_DB_USER}:${COA_DB_PASSWORD}@${COA_DB_HOST}/${COA_DB_NAME}?sslmode=${COA_DB_SSLMODE} up

# revert the last applied migration
migrate-down:
    migrate -path migrations -database postgres://${COA_DB_USER}:${COA_DB_PASSWORD}@${COA_DB_HOST}/${COA_DB_NAME}?sslmode=${COA_DB_SSLMODE} down 1

build:
    go build -o dist github.com/haikoschol/cats-of-asia/cmd/coabot
    go build -o dist github.com/haikoschol/cats-of-asia/cmd/ingest
    go build -o dist github.com/haikoschol/cats-of-asia/cmd/web

build-linux:
    GOOS=linux GOARCH=amd64 go build -o dist/linux github.com/haikoschol/cats-of-asia/cmd/coabot

# build the binary if necessary and deploy to hostname (which is assumed to run x86_64 Linux)
deploy hostname: build-linux
    ssh -t {{hostname}} "sudo systemctl stop coabot"
    scp dist/linux/coabot {{hostname}}:/usr/local/bin/coabot
    ssh -t {{hostname}} "sudo systemctl start coabot"
