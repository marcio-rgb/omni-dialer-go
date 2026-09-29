package postgres

import (
	"strings"
	"testing"
)

func TestBuildSmartSearchClause_FormattedPhone(t *testing.T) {
	// Telefone com caracteres de formatação deve ser limpo e tratado como busca numérica
	raw := "(11) 98765-4321"
	res, nextIdx := buildSmartSearchClause(raw, 2)
	if res == nil {
		t.Fatalf("esperava cláusula gerada")
	}

	if !strings.Contains(res.SQLClause, "phone = $2") {
		t.Errorf("esperava busca por phone exato na cláusula, obteve: %s", res.SQLClause)
	}
	if len(res.Args) != 3 {
		t.Fatalf("esperado 3 args, obteve %d", len(res.Args))
	}
	if res.Args[0] != "11987654321" {
		t.Errorf("dígitos limpos incorretos: %v", res.Args[0])
	}
	if nextIdx != 5 {
		t.Errorf("nextIdx incorreto: %d", nextIdx)
	}
}

func TestBuildSmartSearchClause_FormattedCPF(t *testing.T) {
	raw := "123.456.789-00"
	res, _ := buildSmartSearchClause(raw, 3)
	if res == nil {
		t.Fatalf("esperava cláusula gerada")
	}
	if !strings.Contains(res.SQLClause, "lead_cpf = $5") {
		t.Errorf("esperava busca por lead_cpf, obteve: %s", res.SQLClause)
	}
	if res.Args[0] != "12345678900" {
		t.Errorf("dígitos limpos do CPF incorretos: %v", res.Args[0])
	}
}

func TestBuildSmartSearchClause_PartialPrefix(t *testing.T) {
	raw := "119876" // 6 dígitos: busca parcial por prefixo/sufixo
	res, _ := buildSmartSearchClause(raw, 2)
	if res == nil {
		t.Fatalf("esperava cláusula gerada")
	}
	if !strings.Contains(res.SQLClause, "LIKE") {
		t.Errorf("esperava cláusula com LIKE, obteve: %s", res.SQLClause)
	}
}

func TestBuildSmartSearchClause_TextSearch(t *testing.T) {
	raw := "atendimento cancelamento"
	res, _ := buildSmartSearchClause(raw, 4)
	if res == nil {
		t.Fatalf("esperava cláusula gerada")
	}
	if !strings.Contains(res.SQLClause, "websearch_to_tsquery") {
		t.Errorf("esperava websearch_to_tsquery, obteve: %s", res.SQLClause)
	}
	if res.Args[0] != "atendimento cancelamento" {
		t.Errorf("argumento de texto incorreto: %v", res.Args[0])
	}
}
