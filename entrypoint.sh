#!/bin/sh
set -e

echo "Aguardando o PostgreSQL (${DB_HOST}:${DB_PORT}) ficar disponível..."
while ! nc -z "$DB_HOST" "$DB_PORT"; do
    echo "PostgreSQL ainda não respondeu. Aguarde..."
    sleep 1
done
echo "PostgreSQL disponível!"

export PGPASSWORD="$DB_PASSWORD"

echo "Executando migrações..."
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f /app/migrations/0001_create_users_table.up.sql

echo "Verificando se existe ao menos um usuário na tabela 'users'..."
user_count=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM users;")
user_count=$(echo $user_count | xargs)

if [ "$user_count" -eq "0" ]; then
    echo "Nenhum usuário encontrado. Inserindo usuário padrão..."
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "INSERT INTO users (name, token) VALUES ('John','1234ABCD');"
else
    echo "Usuário(s) encontrado(s): $user_count."
fi

echo "Iniciando o wuzapi com o log em formato json..."
exec /app/wuzapi -logtype json "$@"
