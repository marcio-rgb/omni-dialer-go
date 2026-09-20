package domain

import "time"

// Tenant representa um contratante/organização no ecossistema do Dialer-Go.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Webhook   string    `json:"webhook"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InjectLeadParams representa os parâmetros de consulta enviados via GET para o webhook do tenant ao conectar a chamada.
type InjectLeadParams struct {
	UserID string `json:"user_id"`
	CPF    string `json:"cpf"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Att1   string `json:"att1"`
	Att2   string `json:"att2"`
	Att3   string `json:"att3"`
}
