package repository

import (
	"log"

	"github.com/jmoiron/sqlx"
)

// PostgresRepository é a implementação do repositório para PostgreSQL
type PostgresRepository struct {
    db *sqlx.DB
}

// NewPostgresRepository cria uma nova instância do PostgresRepository
func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
    return &PostgresRepository{db: db}
}

// GetAllUsers retorna todos os usuários do banco de dados
// Implementação de exemplo que consulta todos os registros da tabela users
func (r *PostgresRepository) GetAllUsers() ([]User, error) {
    var users []User
    query := "SELECT * FROM users"
    err := r.db.Select(&users, query)
    if err != nil {
        log.Printf("Erro ao buscar usuários: %v", err)
        return nil, err
    }
    return users, nil
}

// User representa a estrutura de dados de um usuário
type User struct {
    ID        int    `db:"id"`
    Name      string `db:"name"`
    Token     string `db:"token"`
    Webhook   string `db:"webhook"`
    Jid       string `db:"jid"`
    Qrcode    string `db:"qrcode"`
    Connected int    `db:"connected"`
    Expiration int   `db:"expiration"`
    Events    string `db:"events"`
}
