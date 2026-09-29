package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

/**
 * Avaliador de Matriz de Confusão do Sistema Legado contra o Ground Truth.
 *
 * @pattern Repository / Quality Assurance
 * @governedBy .agents/workflows/agente-audit-processo.md
 */

type ConfusionMatrix struct {
	TotalAudited int
	TP           int // True Positive: Legado=Machine, Real=Machine
	FP           int // False Positive: Legado=Machine, Real=Human (Derrubou humano!)
	FN           int // False Negative: Legado=Human, Real=Machine (Vazou secretária!)
	TN           int // True Negative: Legado=Human, Real=Human (Entregou humano)
}

func (cm *ConfusionMatrix) PrintReport() {
	precision := 0.0
	if cm.TP+cm.FP > 0 {
		precision = float64(cm.TP) / float64(cm.TP+cm.FP) * 100
	}
	recall := 0.0
	if cm.TP+cm.FN > 0 {
		recall = float64(cm.TP) / float64(cm.TP+cm.FN) * 100
	}
	accuracy := 0.0
	if cm.TotalAudited > 0 {
		accuracy = float64(cm.TP+cm.TN) / float64(cm.TotalAudited) * 100
	}
	fpRate := 0.0
	if cm.FP+cm.TN > 0 {
		fpRate = float64(cm.FP) / float64(cm.FP+cm.TN) * 100
	}
	fnRate := 0.0
	if cm.FN+cm.TP > 0 {
		fnRate = float64(cm.FN) / float64(cm.FN+cm.TP) * 100
	}

	fmt.Println("================================================================================")
	fmt.Println("📊 AUDITORIA DE ACURÁCIA & MATRIZ DE CONFUSÃO (SISTEMA LEGADO)")
	fmt.Println("================================================================================")
	fmt.Printf("Total de Chamadas com Áudio Auditadas: %d\n", cm.TotalAudited)
	fmt.Printf("  - Verdadeiros Positivos (TP - Caixas Postais descartadas corretamente): %d\n", cm.TP)
	fmt.Printf("  - Falsos Positivos     (FP - HUMANOS DERRUBADOS COMO CAIXA POSTAL!):  %d (%.2f%% dos humanos)\n", cm.FP, fpRate)
	fmt.Printf("  - Falsos Negativos     (FN - Caixas postais que vazaram para operador): %d (%.2f%% dos robôs)\n", cm.FN, fnRate)
	fmt.Printf("  - Verdadeiros Negativos (TN - Humanos entregues com sucesso):         %d\n", cm.TN)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("Acurácia Global do Sistema Legado: %.2f%%\n", accuracy)
	fmt.Printf("Precisão de Detecção de Máquina:    %.2f%%\n", precision)
	fmt.Printf("Revocação (Recall de Máquina):     %.2f%%\n", recall)
	fmt.Println("================================================================================")
}

func EvaluateLegacySystem(dbURL string) (*ConfusionMatrix, error) {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao postgres: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	query := `
		SELECT disposition, transcription
		FROM cdrs
		WHERE created_at::date = '2026-09-14'
		  AND transcription IS NOT NULL
		  AND transcription != ''
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar CDRs: %w", err)
	}
	defer rows.Close()

	cm := &ConfusionMatrix{}

	for rows.Next() {
		var disposition, transcription string
		if err := rows.Scan(&disposition, &transcription); err != nil {
			continue
		}

		groundTruthStatus, _ := ClassifyCall(CallMetrics{FullText: transcription})
		cm.TotalAudited++

		isLegacyMachine := (disposition == "VOICEMAIL")
		isLegacyHuman := (disposition == "DELIVERED" || disposition == "ABANDONED" || disposition == "ANSWERED")

		if isLegacyMachine {
			if groundTruthStatus == "MACHINE" {
				cm.TP++
			} else {
				cm.FP++ // O sistema antigo derrubou achando que era secretária, mas era humano!
			}
		} else if isLegacyHuman {
			if groundTruthStatus == "MACHINE" {
				cm.FN++ // O sistema antigo achou que era humano, mas era secretária!
			} else {
				cm.TN++
			}
		}
	}

	cm.PrintReport()
	return cm, nil
}

func UpdateCDRTranscription(db *sql.DB, recordingFile, transcription string) error {
	query := `
		UPDATE cdrs
		SET transcription = $1
		WHERE recording_file LIKE '%' || $2 || '%'
	`
	_, err := db.Exec(query, transcription, recordingFile)
	return err
}

func ConnectDB(dbURL string) (*sql.DB, error) {
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	return sql.Open("pgx", dbURL)
}
