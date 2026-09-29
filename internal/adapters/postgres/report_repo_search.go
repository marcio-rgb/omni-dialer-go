package postgres

import (
	"fmt"
	"strings"
	"unicode"
)

// SearchClauseResult encapsula a cláusula WHERE e os argumentos gerados para a busca inteligente.
type SearchClauseResult struct {
	SQLClause string
	Args      []any
}

// buildSmartSearchClause analisa o termo de pesquisa e decide dinamicamente entre
// busca numérica de alta performance (B-Tree com varchar_pattern_ops) ou busca textual completa (GIN FTS).
//
// @pattern Strategy / Discriminator Pattern
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
//
// @preExecution
// - Termo de busca não vazio
// - argIdx atual para indexação de parâmetros $N do pgx
//
// @postExecution
// - Retorna cláusula SQL parametrizada e argumentos correspondentes
func buildSmartSearchClause(rawTerm string, startArgIdx int) (*SearchClauseResult, int) {
	trimmed := strings.TrimSpace(rawTerm)
	if trimmed == "" {
		return nil, startArgIdx
	}

	cleanDigits, isPureNumber := extractDigitsIfNumeric(trimmed)

	// Caso 1: Termo numérico ou formatado como telefone/CPF (ex: "(11) 99999-8888", "123.456.789-00")
	if isPureNumber && len(cleanDigits) >= 4 {
		switch {
		case len(cleanDigits) <= 9:
			// Busca parcial (prefixo ou sufixo) aproveitando índice varchar_pattern_ops
			clause := fmt.Sprintf("(phone LIKE $%d || '%%' OR phone LIKE '%%' || $%d OR lead_cpf LIKE $%d || '%%')", startArgIdx, startArgIdx+1, startArgIdx+2)
			return &SearchClauseResult{
				SQLClause: clause,
				Args:      []any{cleanDigits, cleanDigits, cleanDigits},
			}, startArgIdx + 3

		case len(cleanDigits) == 10 || len(cleanDigits) == 11:
			// Telefone brasileiro com DDD ou CPF completo
			withCountryCode := "55" + cleanDigits
			clause := fmt.Sprintf("(phone = $%d OR phone = $%d OR lead_cpf = $%d)", startArgIdx, startArgIdx+1, startArgIdx+2)
			return &SearchClauseResult{
				SQLClause: clause,
				Args:      []any{cleanDigits, withCountryCode, cleanDigits},
			}, startArgIdx + 3

		default:
			// 12 ou mais dígitos (telefone internacional ou com DDI 55)
			withoutCountryCode := cleanDigits
			if strings.HasPrefix(cleanDigits, "55") && len(cleanDigits) > 2 {
				withoutCountryCode = cleanDigits[2:]
			}
			clause := fmt.Sprintf("(phone = $%d OR phone = $%d OR lead_cpf = $%d)", startArgIdx, startArgIdx+1, startArgIdx+2)
			return &SearchClauseResult{
				SQLClause: clause,
				Args:      []any{cleanDigits, withoutCountryCode, cleanDigits},
			}, startArgIdx + 3
		}
	}

	// Caso 2: Busca textual em transcrição de áudio via Full-Text Search (GIN)
	// Usa websearch_to_tsquery para suportar frases exatas com aspas e operadores seguros
	clause := fmt.Sprintf(
		"to_tsvector('portuguese', COALESCE(transcription, '')) @@ websearch_to_tsquery('portuguese', $%d)",
		startArgIdx,
	)
	return &SearchClauseResult{
		SQLClause: clause,
		Args:      []any{trimmed},
	}, startArgIdx + 1
}

// extractDigitsIfNumeric remove caracteres de formatação telefônica/documental.
// Se restarem apenas dígitos (com ao menos 4 dígitos), identifica como busca numérica.
func extractDigitsIfNumeric(s string) (string, bool) {
	var sb strings.Builder
	hasLetters := false

	for _, r := range s {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else if unicode.IsLetter(r) {
			hasLetters = true
			break
		}
		// Ignora caracteres de formatação comuns: '(', ')', '-', '.', '/', '+', ' '
	}

	if hasLetters {
		return "", false
	}

	digits := sb.String()
	if len(digits) >= 4 {
		return digits, true
	}

	return "", false
}

// resolveSortColumn valida e resolve a coluna de ordenação com base em uma whitelist segura.
func resolveSortColumn(sortBy string) string {
	switch sortBy {
	case "id", "tenant_id", "campaign_id", "agent_id", "trunk_used",
		"phone", "lead_name", "lead_cpf", "lead_id",
		"call_type", "disposition", "sip_status", "hangup_cause", "amd_status", "amd_cause",
		"duration_seconds", "billsec_seconds", "ring_seconds",
		"created_at", "initiated_at", "answered_at", "ended_at",
		"recording_file", "recording_url", "transcription":
		return sortBy
	default:
		return "created_at"
	}
}
